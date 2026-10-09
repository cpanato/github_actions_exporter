package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1" // nolint: gosec
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/cpanato/github_actions_exporter/model"
	"github.com/google/go-github/v92/github"
)

// WorkflowMetricsExporter struct to hold some information
type WorkflowMetricsExporter struct {
	GHClient           *github.Client
	Logger             *slog.Logger
	Opts               Opts
	PrometheusObserver WorkflowObserver
}

func NewWorkflowMetricsExporter(logger *slog.Logger, opts Opts) *WorkflowMetricsExporter {
	return &WorkflowMetricsExporter{
		Logger:             logger,
		Opts:               opts,
		PrometheusObserver: &PrometheusObserver{},
	}
}

// maxWebhookBodyBytes is the maximum payload size GitHub delivers for a webhook (25 MB).
const maxWebhookBodyBytes = 25 << 20

// HandleGHWebHook responds to POST /gh_event, when it receives an event from GitHub.
func (c *WorkflowMetricsExporter) HandleGHWebHook(w http.ResponseWriter, r *http.Request) {
	buf, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.Logger.Error("webhook body too large", "limit", maxBytesErr.Limit)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		c.Logger.Error("error reading body", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	err = validateSignature(c.Opts.GitHubToken, r.Header, buf)
	if err != nil {
		c.Logger.Error("invalid webhook signature", "err", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	c.Logger.Debug("received webhook", "contentType", r.Header.Get("Content-Type"), "payload", string(buf))

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "ping":
		pingEvent := model.PingEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		if pingEvent == nil {
			c.Logger.Error("unable to decode the ping event")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.Logger.Info("ping event", "hookID", pingEvent.GetHookID())
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status": "honk"}`))
		return
	case "workflow_job":
		event := model.WorkflowJobEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		if event == nil {
			c.Logger.Info("Workflow event is nil due to decoding issues")
			return
		}
		c.Logger.Info("got workflow_job event",
			"org", event.GetRepo().GetOwner().GetLogin(),
			"repo", event.GetRepo().GetName(),
			"branch", event.GetWorkflowJob().GetHeadBranch(),
			"runId", event.GetWorkflowJob().GetRunID(),
			"action", event.GetAction(),
			"workflow_name", event.GetWorkflowJob().GetWorkflowName(),
			"job_name", event.GetWorkflowJob().GetName())
		go c.CollectWorkflowJobEvent(event)
	case "workflow_run":
		event := model.WorkflowRunEventFromJSON(io.NopCloser(bytes.NewBuffer(buf)))
		c.Logger.Info("got workflow_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "branch", event.GetWorkflowRun().GetHeadBranch(), "workflow_name", event.GetWorkflow().GetName(), "runNumber", event.GetWorkflowRun().GetRunNumber(), "action", event.GetAction())
		go c.CollectWorkflowRunEvent(event)
	default:
		c.Logger.Info("not implemented", "eventType", eventType)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

func (c *WorkflowMetricsExporter) CollectWorkflowJobEvent(event *github.WorkflowJobEvent) {
	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	branch := event.WorkflowJob.GetHeadBranch()
	action := event.GetAction()

	workflowJob := event.GetWorkflowJob()
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
			c.Logger.Debug("unable to calculate job duration of in_progress event as event has no steps")
			break
		}

		if len(workflowJob.Steps) > 1 {
			// If there are more than one steps, we are receiving an update of an already running job.
			// Don't count the queued time again since it's already running.
			break
		}

		firstStep := workflowJob.Steps[0]
		queuedSeconds := firstStep.StartedAt.Time.Sub(workflowJob.GetStartedAt().Time).Seconds()
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, branch, "queued", runnerGroup, workflowName, jobName, math.Max(0, queuedSeconds))
	case "completed":
		if workflowJob.StartedAt == nil || workflowJob.CompletedAt == nil {
			c.Logger.Debug("unable to calculate job duration of completed event steps are missing timestamps")
			break
		}

		jobSeconds := math.Max(0, workflowJob.GetCompletedAt().Time.Sub(workflowJob.GetStartedAt().Time).Seconds())
		c.PrometheusObserver.ObserveWorkflowJobDuration(org, repo, branch, "in_progress", runnerGroup, workflowName, jobName, jobSeconds)
		c.PrometheusObserver.CountWorkflowJobDuration(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName, jobSeconds)
	}

	c.PrometheusObserver.CountWorkflowJobStatus(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName)
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

// validateSignature validates the HMAC signature of an incoming GitHub event.
// X-Hub-Signature-256 is preferred, the legacy SHA-1 X-Hub-Signature is only used when it is absent.
func validateSignature(gitHubToken string, headers http.Header, body []byte) error {
	var (
		algorithm string
		newHash   func() hash.Hash
		header    string
	)
	if value := headers.Get("X-Hub-Signature-256"); value != "" {
		algorithm, newHash, header = "sha256", sha256.New, value
	} else if value := headers.Get("X-Hub-Signature"); value != "" {
		algorithm, newHash, header = "sha1", sha1.New, value
	} else {
		return errors.New("missing X-Hub-Signature-256 or X-Hub-Signature header")
	}

	receivedAlgorithm, receivedHex, found := strings.Cut(header, "=")
	if !found || receivedAlgorithm != algorithm {
		return fmt.Errorf("malformed signature header, expected %s=<hex digest>", algorithm)
	}

	received, err := hex.DecodeString(receivedHex)
	if err != nil {
		return fmt.Errorf("signature is not a valid hex digest: %w", err)
	}

	mac := hmac.New(newHash, []byte(gitHubToken))
	mac.Write(body)
	if !hmac.Equal(received, mac.Sum(nil)) {
		return errors.New("signature does not match the payload")
	}

	return nil
}
