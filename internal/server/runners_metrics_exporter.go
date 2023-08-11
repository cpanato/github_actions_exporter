package server

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/go-github/v47/github"
)

type RunnersMetricsExporter struct {
	GHClient GitHubClient
	Logger   log.Logger
	Opts     Opts
	Observer RunnersObserver
}

func NewRunnersMetricsExporter(logger log.Logger, opts Opts, client GitHubClient, observer RunnersObserver) *RunnersMetricsExporter {
	return &RunnersMetricsExporter{
		Logger:   logger,
		Opts:     opts,
		GHClient: client,
		Observer: observer,
	}
}

func (c *RunnersMetricsExporter) Start(ctx context.Context) error {
	if c.Opts.GitHubOrg == "" {
		return errors.New("github org not configured")
	}
	if c.Opts.GitHubAPIToken == "" {
		return errors.New("github token not configured")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.RunnersAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectRunnersInformation(ctx)
			case <-ctx.Done():
				_ = level.Info(c.Logger).Log("msg", "stopped polling for runner metrics")
				return
			}
		}
	}()

	return nil
}

func (c *RunnersMetricsExporter) collectRunnersInformation(ctx context.Context) {
	// Resetting, otherwise a certain label combination might retain its old value despite not being present in the pool
	// For example, if there are no busy runners then group[true] will be empty and the old value of group[true] will
	// continue to be reported rather than set to 0 as expected. Same would be true if API calls fail so we reset first.
	c.Observer.ResetRegisteredRunnersTotal()

	allRunners := make(map[string][]*github.Runner)
	runnerGroups, err := c.GHClient.GetOrganisationRunnerGroups(ctx, c.Opts.GitHubOrg)

	if err != nil {
		_ = level.Error(c.Logger).Log("msg", "unable to retrieve runner groups", "error", err.Error())
		return
	}

	for _, runnerGroup := range runnerGroups {
		groupRunners, err := c.GHClient.GetGroupRunners(ctx, *runnerGroup.ID, c.Opts.GitHubOrg)

		if err != nil {
			_ = level.Error(c.Logger).Log("msg", "unable to retrieve organisation runners' info", "error", err.Error())
			return
		}

		allRunners[*runnerGroup.Name] = groupRunners
	}

	// Collect information from the Enterprise runners, if an Enterprise name has been configured.
	// Requires the GitHub API Token to have manage_runners:enterprise scope.
	if c.Opts.GitHubEnterprise != "" {
		enterpriseRunners, err := c.GHClient.GetEnterpriseRunners(ctx, c.Opts.GitHubEnterprise)

		// We are putting the enterprise runners into a fake runner group named after the enterprise
		// This is because we already have that name in Grafana and also because there is no way in the API at the moment
		// to tie them to their real runner group
		allRunners[c.Opts.GitHubEnterprise] = enterpriseRunners

		if err != nil {
			_ = level.Error(c.Logger).Log("msg", "unable to retrieve enterprise runners' info", "error", err.Error())
			return
		}
	}

	for group, runners := range allRunners {
		for _, runner := range runners {
			c.Observer.IncreaseRegisteredRunnersTotal(runner.GetBusy(), runner.GetStatus(), group)
		}
	}
}
