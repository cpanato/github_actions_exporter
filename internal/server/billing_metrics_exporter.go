package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/prometheus/client_golang/prometheus"
)

type BillingMetricsExporter struct {
	GHClient *github.Client
	Logger   *slog.Logger
	Opts     Opts
}

func NewBillingMetricsExporter(logger *slog.Logger, opts Opts) *BillingMetricsExporter {
	var clientOpts []github.ClientOptionsFunc
	// WithAuthToken rejects empty tokens; billing polling is disabled without one anyway.
	if opts.GitHubAPIToken != "" {
		clientOpts = append(clientOpts, github.WithAuthToken(opts.GitHubAPIToken))
	}
	client, err := github.NewClient(clientOpts...)
	if err != nil {
		logger.Error("failed to create the GitHub client for billing", "err", err)
	}

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
	if c.GHClient == nil {
		return errors.New("github client not initialized")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectOrgBilling(ctx)
			case <-ctx.Done():
				c.Logger.Info("stopped polling for org billing metrics")
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
	if c.GHClient == nil {
		return errors.New("github client not initialized")
	}

	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectUserBilling(ctx)
			case <-ctx.Done():
				ticker.Stop()
				c.Logger.Info("stopped polling for user billing metrics")
				return
			}
		}
	}()

	return nil
}

// collectOrgBilling collects the Actions usage of the current month for an org.
func (c *BillingMetricsExporter) collectOrgBilling(ctx context.Context) {
	report, _, err := c.GHClient.Billing.GetOrganizationUsageReport(ctx, c.Opts.GitHubOrg, currentMonthUsageOptions(time.Now()))
	if err != nil {
		c.Logger.Error("failed to retrieve the actions billing for an org", "org", c.Opts.GitHubOrg, "err", err)
		return
	}

	setActionsUsageMetrics(c.Opts.GitHubOrg, "", summarizeActionsUsage(report.UsageItems))
}

func (c *BillingMetricsExporter) collectUserBilling(ctx context.Context) {
	report, _, err := c.GHClient.Billing.GetUsageReport(ctx, c.Opts.GitHubUser, currentMonthUsageOptions(time.Now()))
	if err != nil {
		c.Logger.Error("failed to retrieve the actions billing for an user", "user", c.Opts.GitHubUser, "err", err)
		return
	}

	setActionsUsageMetrics("", c.Opts.GitHubUser, summarizeActionsUsage(report.UsageItems))
}

func currentMonthUsageOptions(now time.Time) *github.UsageReportOptions {
	now = now.UTC()
	return &github.UsageReportOptions{
		Year:  new(now.Year()),
		Month: new(int(now.Month())),
	}
}

// actionsUsage is the Actions minutes usage aggregated from a billing usage report.
type actionsUsage struct {
	// totalMinutes is every minute used, whether it was billed or not.
	totalMinutes float64
	// paidMinutes is the part of totalMinutes that was actually charged.
	paidMinutes float64
	// minutesByHostType is totalMinutes split by runner type.
	minutesByHostType map[string]float64
}

// legacyHostTypes keeps the host_type label values of the retired Actions billing API
// for the standard runners.
var legacyHostTypes = map[string]string{
	"actions_linux":   "UBUNTU",
	"actions_windows": "WINDOWS",
	"actions_macos":   "MACOS",
}

func summarizeActionsUsage(items []*github.UsageItem) actionsUsage {
	usage := actionsUsage{minutesByHostType: map[string]float64{}}
	for _, item := range items {
		if item == nil || !strings.EqualFold(item.Product, "actions") || !strings.EqualFold(item.UnitType, "minutes") {
			continue
		}

		usage.totalMinutes += item.Quantity
		if item.PricePerUnit > 0 {
			usage.paidMinutes += min(max(item.NetAmount/item.PricePerUnit, 0), item.Quantity)
		}

		sku := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.SKU), " ", "_"))
		hostType, ok := legacyHostTypes[sku]
		if !ok {
			hostType = sku
		}
		usage.minutesByHostType[hostType] += item.Quantity
	}

	return usage
}

func setActionsUsageMetrics(org, user string, usage actionsUsage) {
	totalMinutesUsedActions.WithLabelValues(org, user).Set(usage.totalMinutes)
	includedMinutesUsedActions.WithLabelValues(org, user).Set(usage.totalMinutes - usage.paidMinutes)
	totalPaidMinutesActions.WithLabelValues(org, user).Set(usage.paidMinutes)

	// Drop host types that are no longer reported, e.g. after the month rolls over.
	totalMinutesUsedByHostTypeActions.DeletePartialMatch(prometheus.Labels{"org": org, "user": user})
	for hostType, minutes := range usage.minutesByHostType {
		totalMinutesUsedByHostTypeActions.WithLabelValues(org, user, hostType).Set(minutes)
	}
}
