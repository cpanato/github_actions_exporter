package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log/level"
)

func (c *ExporterClient) StartOrgBilling(ctx context.Context) error {
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

func (c *ExporterClient) StartUserBilling(ctx context.Context) error {
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
func (c *ExporterClient) collectOrgBilling(ctx context.Context) {
	actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingOrg(ctx, c.Opts.GitHubOrg)
	if err != nil {
		_ = c.Logger.Log("msg", "failed to retrieve the actions billing for an org", "org", c.Opts.GitHubOrg, "err", err)
		return
	}

	totalMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(actionsBilling.TotalMinutesUsed)
	includedMinutesUsedActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(actionsBilling.IncludedMinutes)
	totalPaidMinutesActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(actionsBilling.TotalPaidMinutesUsed)

	for host, minutes := range actionsBilling.MinutesUsedBreakdown {
		totalMinutesUsedByHostTypeActions.WithLabelValues(c.Opts.GitHubOrg, "", host).Set(float64(minutes))
	}

	// TODO: deprecate
	totalMinutesUsedUbuntuActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown["UBUNTU"]))
	totalMinutesUsedMacOSActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown["MACOS"]))
	totalMinutesUsedWindowsActions.WithLabelValues(c.Opts.GitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown["WINDOWS"]))
}

func (c *ExporterClient) collectUserBilling(ctx context.Context) {
	actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingUser(ctx, c.Opts.GitHubUser)
	if err != nil {
		_ = c.Logger.Log("msg", "failed to retrieve the actions billing for an user", "user", c.Opts.GitHubUser, "err", err)
		return
	}

	totalMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(actionsBilling.TotalMinutesUsed)
	includedMinutesUsedActions.WithLabelValues("", c.Opts.GitHubUser).Set(actionsBilling.IncludedMinutes)
	totalPaidMinutesActions.WithLabelValues("", c.Opts.GitHubUser).Set(actionsBilling.TotalPaidMinutesUsed)

	for host, minutes := range actionsBilling.MinutesUsedBreakdown {
		totalMinutesUsedByHostTypeActions.WithLabelValues("", c.Opts.GitHubUser, host).Set(float64(minutes))
	}

	// TODO: deprecate
	totalMinutesUsedUbuntuActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown["UBUNTU"]))
	totalMinutesUsedMacOSActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown["MACOS"]))
	totalMinutesUsedWindowsActions.WithLabelValues("", c.Opts.GitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown["WINDOWS"]))
}
