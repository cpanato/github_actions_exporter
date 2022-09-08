package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1" // nolint: gosec
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strings"

	"github.com/cpanato/github_actions_exporter/model"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/go-github/v43/github"
)

// WorkflowMetricsExporter struct to hold some information
type WorkflowMetricsExporter struct {
	GHClient           *github.Client
	Logger             log.Logger
	Opts               Opts
	PrometheusObserver WorkflowObserver
}

func NewWorkflowMetricsExporter(logger log.Logger, opts Opts) *WorkflowMetricsExporter {
	return &WorkflowMetricsExporter{
		Logger:             logger,
		Opts:               opts,
		PrometheusObserver: &PrometheusObserver{},
	}
}

// handleGHWebHook responds to POST /gh_event, when receive a event from GitHub.
func (c *WorkflowMetricsExporter) HandleGHWebHook(w http.ResponseWriter, r *http.Request) {
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		_ = level.Error(c.Logger).Log("msg", "error reading body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	receivedHash := strings.SplitN(r.Header.Get("X-Hub-Signature"), "=", 2)
	if receivedHash[0] != "sha1" {
		_ = level.Error(c.Logger).Log("msg", "invalid webhook hash signature: SHA1")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = validateSignature(c.Opts.GitHubToken, receivedHash, buf)
	if err != nil {
		_ = level.Error(c.Logger).Log("msg", "invalid token", "err", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	_ = level.Debug(c.Logger).Log("msg", "received webhook", "payload", string(buf))

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "ping":
		pingEvent := model.PingEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		if pingEvent == nil {
			_ = level.Info(c.Logger).Log("msg", "ping event", "hookID", pingEvent.GetHookID())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status": "honk"}`))
		return
	case "workflow_job":
		event := model.WorkflowJobEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		_ = level.Info(c.Logger).Log("msg", "got workflow_job event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "runId", event.GetWorkflowJob().GetRunID(), "action", event.GetAction())
		go c.CollectWorkflowJobEvent(event)
	case "workflow_run":
		event := model.WorkflowRunEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		_ = level.Info(c.Logger).Log("msg", "got workflow_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "workflow_name", event.GetWorkflow().GetName(), "runNumber", event.GetWorkflowRun().GetRunNumber(), "action", event.GetAction())
		go c.CollectWorkflowRunEvent(event)
	default:
		_ = level.Info(c.Logger).Log("msg", "not implemented", "eventType", eventType)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

func (c *WorkflowMetricsExporter) CollectWorkflowJobEvent(event *github.WorkflowJobEvent) {
	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	runnerGroup := event.WorkflowJob.GetRunnerGroupName()

	action := event.GetAction()
	conclusion := event.GetWorkflowJob().GetConclusion()
	status := event.GetWorkflowJob().GetStatus()

	switch action {
	case "queued":
		// Do nothing.
	case "in_progress":

		if len(event.WorkflowJob.Steps) == 0 {
			_ = level.Debug(c.Logger).Log("msg", "unable to calculate job duration of in_progress event as event has no steps")
			break
		}

		firstStep := event.WorkflowJob.Steps[0]
		queuedSeconds := firstStep.StartedAt.Time.Sub(event.WorkflowJob.StartedAt.Time).Seconds()
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, "queued", runnerGroup, math.Max(0, queuedSeconds))
	case "completed":
		if event.WorkflowJob.StartedAt == nil || event.WorkflowJob.CompletedAt == nil {
			_ = level.Debug(c.Logger).Log("msg", "unable to calculate job duration of completed event steps are missing timestamps")
			break
		}

		jobSeconds := math.Max(0, event.WorkflowJob.GetCompletedAt().Time.Sub(event.WorkflowJob.GetStartedAt().Time).Seconds())
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, "in_progress", runnerGroup, jobSeconds)
		c.PrometheusObserver.CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup, jobSeconds)
	}

	c.PrometheusObserver.CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup)
}

func (c *WorkflowMetricsExporter) CollectWorkflowRunEvent(event *github.WorkflowRunEvent) {
	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	workflowName := event.GetWorkflow().GetName()

	if event.GetAction() == "completed" {
		seconds := event.GetWorkflowRun().UpdatedAt.Time.Sub(event.GetWorkflowRun().RunStartedAt.Time).Seconds()
		c.PrometheusObserver.ObserveWorkflowRunDuration(org, repo, workflowName, seconds)
	}

	status := event.GetWorkflowRun().GetStatus()
	conclusion := event.GetWorkflowRun().GetConclusion()
	c.PrometheusObserver.CountWorkflowRunStatus(org, repo, status, conclusion, workflowName)
}

// validateSignature validate the incoming github event.
func validateSignature(gitHubToken string, receivedHash []string, bodyBuffer []byte) error {
	hash := hmac.New(sha1.New, []byte(gitHubToken))
	if _, err := hash.Write(bodyBuffer); err != nil {
		msg := fmt.Sprintf("Cannot compute the HMAC for request: %s\n", err)
		return errors.New(msg)
	}

	expectedHash := hex.EncodeToString(hash.Sum(nil))
	if receivedHash[1] != expectedHash {
		msg := fmt.Sprintf("Expected Hash does not match the received hash: %s\n", expectedHash)
		return errors.New(msg)
	}

	return nil
}
