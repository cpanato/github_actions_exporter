package server

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type BillingMetricsExporter struct {
	GHClient GitHubClient
	Logger   log.Logger
	Opts     Opts
}

func NewBillingMetricsExporter(logger log.Logger, opts Opts, client GitHubClient) *BillingMetricsExporter {
	return &BillingMetricsExporter{
		Logger:   logger,
		Opts:     opts,
		GHClient: client,
	}
}

func (c *BillingMetricsExporter) StartOrgBilling(ctx context.Context) error {
	if c.Opts.GitHubOrg == "" {
		return errors.New("github org not configured")
	}
	if c.Opts.GitHubAPIToken == "" {
		return errors.New("github token not configured")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectOrgBilling(ctx)
			case <-ctx.Done():
				_ = level.Info(c.Logger).Log("msg", "stopped polling for org billing metrics")
				return
			}
		}
	}()

	return nil
}

func (c *BillingMetricsExporter) StartUserBilling(ctx context.Context) error {
	if c.Opts.GitHubUser == "" {
		return errors.New("github user not configured")
	}
	if c.Opts.GitHubAPIToken == "" {
		return errors.New("github token not configured")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectUserBilling(ctx)
			case <-ctx.Done():
				ticker.Stop()
				_ = level.Info(c.Logger).Log("msg", "stopped polling for user billing metrics")
				return
			}
		}
	}()

	return nil
}

// CollectActionBilling collect the action billing.
func (c *BillingMetricsExporter) collectOrgBilling(ctx context.Context) {
	actionsBilling, err := c.GHClient.GetActionsBillingOrg(ctx, c.Opts.GitHubOrg)
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

func (c *BillingMetricsExporter) collectUserBilling(ctx context.Context) {
	actionsBilling, err := c.GHClient.GetActionsBillingUser(ctx, c.Opts.GitHubUser)
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
