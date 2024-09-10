package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1" // nolint: gosec
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/fernride/github_actions_exporter/model"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/go-github/v59/github"
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
	buf, err := io.ReadAll(r.Body)
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
		pingEvent := model.PingEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		if pingEvent == nil {
			_ = level.Info(c.Logger).Log("msg", "ping event", "hookID", pingEvent.GetHookID())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status": "honk"}`))
		return
	case "workflow_job":
		event := model.WorkflowJobEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		_ = level.Info(c.Logger).Log("msg", "got workflow_job event",
			"org", event.GetRepo().GetOwner().GetLogin(),
			"repo", event.GetRepo().GetName(),
			"runId", event.GetWorkflowJob().GetRunID(),
			"action", event.GetAction(),
			"workflow_name", event.GetWorkflowJob().GetWorkflowName(),
			"job_name", event.GetWorkflowJob().GetName(),
			"branch", event.GetWorkflowJob().GetHeadBranch())
		go c.CollectWorkflowJobEvent(event)
	case "workflow_run":
		event := model.WorkflowRunEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		_ = level.Info(c.Logger).Log("msg", "got workflow_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "branch", event.GetWorkflowRun().GetHeadBranch(), "workflow_name", event.GetWorkflow().GetName(), "runNumber", event.GetWorkflowRun().GetRunNumber(), "action", event.GetAction())
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

	action := event.GetAction()

	workflowJob := event.GetWorkflowJob()
	branch := workflowJob.GetHeadBranch()
	runnerGroup := workflowJob.GetRunnerGroupName()
	conclusion := workflowJob.GetConclusion()
	status := workflowJob.GetStatus()
	workflowName := workflowJob.GetWorkflowName()
	jobName := workflowJob.GetName()

	switch action {
	case "queued":
		// Do nothing.
	case "in_progress":

		if len(workflowJob.Steps) == 0 {
			_ = level.Debug(c.Logger).Log("msg", "unable to calculate job duration of in_progress event as event has no steps")
			break
		}

		firstStep := workflowJob.Steps[0]
		queuedSeconds := firstStep.StartedAt.Time.Sub(workflowJob.GetStartedAt().Time).Seconds()
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, "queued", runnerGroup, workflowName, jobName, branch, math.Max(0, queuedSeconds))
	case "completed":
		if workflowJob.StartedAt == nil || workflowJob.CompletedAt == nil {
			_ = level.Debug(c.Logger).Log("msg", "unable to calculate job duration of completed event steps are missing timestamps")
			break
		}

		jobSeconds := math.Max(0, workflowJob.GetCompletedAt().Time.Sub(workflowJob.GetStartedAt().Time).Seconds())
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, "in_progress", runnerGroup, workflowName, jobName, branch, jobSeconds)
		c.PrometheusObserver.CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch, jobSeconds)
	}

	c.PrometheusObserver.CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch)
}

func (c *WorkflowMetricsExporter) CollectWorkflowRunEvent(event *github.WorkflowRunEvent) {
	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	branch := event.GetWorkflowRun().GetHeadBranch()
	workflowName := event.GetWorkflow().GetName()
	conclusion := event.GetWorkflowRun().GetConclusion()

	if event.GetAction() == "completed" {
		seconds := event.GetWorkflowRun().UpdatedAt.Time.Sub(event.GetWorkflowRun().RunStartedAt.Time).Seconds()
		c.PrometheusObserver.ObserveWorkflowRunDuration(org, repo, branch, workflowName, conclusion, seconds)
	}

	status := event.GetWorkflowRun().GetStatus()
	c.PrometheusObserver.CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflowName)
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
