package server

import (
	"bytes"
	"context"
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
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"
)

var (
	workflowJobHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_job_duration_seconds",
		Help:    "Time that a workflow job took to reach a given state.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "state", "runner_group"},
	)

	workflowRunHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_execution_time_seconds",
		Help:    "Time that a workflow took to run.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "workflow_name"},
	)

	totalMinutesUsedActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_minutes",
		Help: "Total minutes used for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	includedMinutesUsedActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_included_minutes",
		Help: "Included Minutes for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalPaidMinutesActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_paid_minutes",
		Help: "Paid Minutes for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedUbuntuActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_ubuntu_minutes",
		Help: "Total minutes used for Ubuntu type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedMacOSActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_macos_minutes",
		Help: "Total minutes used for MacOS type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)

	totalMinutesUsedWindowsActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_windows_minutes",
		Help: "Total minutes used for Windows type for the GitHub Actions.",
	},
		[]string{"org", "user"},
	)
)

func init() {
	// Register metrics with prometheus
	prometheus.MustRegister(workflowJobHistogramVec)
	prometheus.MustRegister(workflowRunHistogramVec)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedUbuntuActions)
	prometheus.MustRegister(totalMinutesUsedMacOSActions)
	prometheus.MustRegister(totalMinutesUsedWindowsActions)
}

// GHActionExporter struct to hold some information
type GHActionExporter struct {
	GHClient    *github.Client
	Logger      log.Logger
	Opts        Opts
	JobObserver WorkflowJobObserver
}

func NewGHActionExporter(logger log.Logger, opts Opts) *GHActionExporter {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.GitHubAPIToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &GHActionExporter{
		GHClient:    client,
		Logger:      logger,
		Opts:        opts,
		JobObserver: &JobObserver{},
	}
}

// CollectActionBilling collect the action billing.
func (c *GHActionExporter) CollectActionBilling() {
	if c.Opts.GitHubOrg != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingOrg(context.TODO(), c.Opts.GitHubOrg)
		if err != nil {
			_ = c.Logger.Log("msg", "failed to retrieve the actions billing for an org", "org", c.Opts.GitHubOrg, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(actionsBilling.TotalPaidMinutesUsed)
		totalMinutesUsedUbuntuActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}

	if c.Opts.GitHubUser != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingUser(context.TODO(), c.Opts.GitHubUser)
		if err != nil {
			_ = c.Logger.Log("msg", "failed to retrieve the actions billing for an user", "user", c.Opts.GitHubUser, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues("", c.Opts.GitHubUser).Set(actionsBilling.TotalPaidMinutesUsed)
		totalMinutesUsedUbuntuActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}
}

// handleGHWebHook responds to POST /gh_event, when receive a event from GitHub.
func (c *GHActionExporter) HandleGHWebHook(w http.ResponseWriter, r *http.Request) {
	buf, _ := ioutil.ReadAll(r.Body)

	receivedHash := strings.SplitN(r.Header.Get("X-Hub-Signature"), "=", 2)
	if receivedHash[0] != "sha1" {
		_ = level.Error(c.Logger).Log("msg", "invalid webhook hash signature: SHA1")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := validateSignature(c.Opts.GitHubToken, receivedHash, buf)
	if err != nil {
		_ = level.Error(c.Logger).Log("msg", "invalid token", "err", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

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
		level.Info(c.Logger).Log("msg", "got workflow_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "workflow_name", event.GetWorkflow().GetName(), "runNumber", event.GetWorkflowRun().GetRunNumber(), "action", event.GetAction())
		go c.CollectWorkflowRunEvent(event)
	default:
		_ = level.Info(c.Logger).Log("msg", "not implemented", "eventType", eventType)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	go c.CollectActionBilling()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

func (c *GHActionExporter) CollectWorkflowJobEvent(event *github.WorkflowJobEvent) {
	if event.GetAction() == "queued" {
		return
	}

	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	runnerGroup := event.WorkflowJob.GetRunnerGroupName()

	if event.GetAction() == "in_progress" {
		firstStep := event.WorkflowJob.Steps[0]
		queuedSeconds := firstStep.StartedAt.Time.Sub(event.WorkflowJob.StartedAt.Time).Seconds()
		c.JobObserver.ObserveWorkflowJobDuration(org, repo, "queued", runnerGroup, math.Max(0, queuedSeconds))
	}

	if event.GetAction() == "completed" && event.GetWorkflowJob().GetConclusion() != "skipped" {
		firstStepStarted := event.WorkflowJob.Steps[0].StartedAt.Time
		lastStepCompleted := event.WorkflowJob.Steps[len(event.WorkflowJob.Steps)-1].CompletedAt.Time
		jobSeconds := lastStepCompleted.Sub(firstStepStarted).Seconds()
		c.JobObserver.ObserveWorkflowJobDuration(org, repo, "in_progress", runnerGroup, math.Max(0, jobSeconds))
	}
}

func (c *GHActionExporter) CollectWorkflowRunEvent(event *github.WorkflowRunEvent) {
	if event.GetAction() != "completed" {
		return
	}

	repo := event.GetRepo().GetName()
	org := event.GetRepo().GetOwner().GetLogin()
	workflowName := event.GetWorkflow().GetName()
	seconds := event.GetWorkflowRun().UpdatedAt.Time.Sub(event.GetWorkflowRun().RunStartedAt.Time).Seconds()
	c.JobObserver.ObserveWorkflowRunDuration(org, repo, workflowName, float64(seconds))
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
