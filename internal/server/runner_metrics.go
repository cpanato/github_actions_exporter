package server

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/go-github/v50/github"
)

type RunnerMetricsExporter struct {
	GHClient *github.Client
	Logger   log.Logger
	Opts     Opts
}

func NewRunnerMetricsExporter(logger log.Logger, opts Opts) *RunnerMetricsExporter {
	client := getGithubClient(opts.GitHubAPIToken)

	return &RunnerMetricsExporter{
		GHClient: client,
		Logger:   logger,
		Opts:     opts,
	}
}

func (e *RunnerMetricsExporter) StartOrgRunnerMetricsCollection(ctx context.Context) {
	if e.Opts.GitHubOrg == "" {
		_ = level.Info(e.Logger).Log("msg", "Github org is not set, no org runner metrics will be collected.")
		return
	}
	if e.Opts.GitHubAPIToken == "" {
		_ = level.Info(e.Logger).Log("msg", "Github token is not set, no org runner metrics will be collected.")
		return
	}

	ticker := time.NewTicker(time.Duration(e.Opts.RunnersAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				e.collectOrgRunnerMetrics(ctx)
			case <-ctx.Done():
				ticker.Stop()
				_ = level.Info(e.Logger).Log("msg", "Stopped polling for org runner metrics.")
				return
			}
		}
	}()
}

func (e *RunnerMetricsExporter) collectOrgRunnerMetrics(ctx context.Context) {
	runners, _, err := e.GHClient.Actions.ListOrganizationRunners(ctx, e.Opts.GitHubOrg, nil)

	if err != nil {
		_ = e.Logger.Log("msg", "Failed to retrieve org runners for org ", e.Opts.GitHubOrg, " ", err)
	}

	numberOfSelfHostedRunners.WithLabelValues(e.Opts.GitHubOrg).Set(float64(runners.TotalCount))
}
