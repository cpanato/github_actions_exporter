package server

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

type ApiWorkflowMetricsExporter struct {
	GHClient *github.Client
	Logger   log.Logger
	Opts     Opts
}

func NewApiWorkflowMetricsExporter(logger log.Logger, opts Opts) *ApiWorkflowMetricsExporter {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.GitHubAPIToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &ApiWorkflowMetricsExporter{
		Logger:   logger,
		Opts:     opts,
		GHClient: client,
	}
}

func (c *ApiWorkflowMetricsExporter) StartWorkflowApiPolling(ctx context.Context) error {
	if c.Opts.GitHubOrg == "" {
		return errors.New("github org not configured")
	}
	if c.Opts.GitHubAPIToken == "" {
		return errors.New("github token not configured")
	}
	if c.Opts.GitHubRepo == "" {
		return errors.New("github repo not configured")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.WorkflowsAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectWorkflowApiPolling(ctx)
			case <-ctx.Done():
				_ = level.Info(c.Logger).Log("msg", "stopped polling for workflow metrics")
				return
			}
		}
	}()

	return nil
}

// CollectActionBilling collect the action billing.
func (c *ApiWorkflowMetricsExporter) collectWorkflowApiPolling(ctx context.Context) {
	queuedWorkflowRuns, _, err := c.GHClient.Actions.ListRepositoryWorkflowRuns(ctx, c.Opts.GitHubOrg, c.Opts.GitHubRepo, &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
		Status: "queued",
	})

	if err != nil {
		_ = c.Logger.Log("msg", "failed to retrieve workflow metrics for an org", "org", c.Opts.GitHubOrg, "err", err)
		return
	}

	workflowQueueSize.WithLabelValues().Set(float64(queuedWorkflowRuns.GetTotalCount()))
}
