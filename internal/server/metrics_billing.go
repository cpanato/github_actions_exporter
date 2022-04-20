package server

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/google/go-github/v43/github"
)

type BillingMetricsExporter struct {
	GHClient *github.Client
	Logger   log.Logger
	Opts     Opts
}

// CollectActionBilling collect the action billing.
func (c *BillingMetricsExporter) CollectActionBilling(ctx context.Context) {
	if c.Opts.GitHubOrg != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingOrg(ctx, c.Opts.GitHubOrg)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an org", "org", c.Opts.GitHubOrg, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}

	if c.Opts.GitHubUser != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingUser(ctx, c.Opts.GitHubUser)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an user", "user", c.Opts.GitHubUser, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}
}
