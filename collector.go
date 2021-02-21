package main

import (
	"context"

	"github.com/google/go-github/v33/github"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	histogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
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
	//Register metrics with prometheus
	prometheus.MustRegister(histogramVec)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedUbuntuActions)
	prometheus.MustRegister(totalMinutesUsedMacOSActions)
	prometheus.MustRegister(totalMinutesUsedWindowsActions)
}

// CollectWorkflowRun collect the workflow execution run metric
func CollectWorkflowRun(checkRunEvent *github.CheckRunEvent) {
	if checkRunEvent.GetCheckRun().GetStatus() != "completed" {
		return
	}

	workflowName := checkRunEvent.GetCheckRun().GetName()
	repo := checkRunEvent.GetRepo().GetName()
	org := checkRunEvent.GetRepo().GetOwner().GetLogin()
	endTime := checkRunEvent.GetCheckRun().GetCompletedAt()
	startTime := checkRunEvent.GetCheckRun().GetStartedAt()
	executionTime := endTime.Sub(startTime.Time).Seconds()

	histogramVec.WithLabelValues(org, repo, workflowName).Observe(executionTime)
}

// CollectActionBilling collect the action billing.
func (c *GHActionExporter) CollectActionBilling() {
	if *gitHubOrg != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingOrg(context.TODO(), *gitHubOrg)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an org", "org", *gitHubOrg, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues(*gitHubOrg, "").Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}

	if *gitHubUser != "" {
		actionsBilling, _, err := c.GHClient.Billing.GetActionsBillingUser(context.TODO(), *gitHubUser)
		if err != nil {
			c.Logger.Log("msg", "failed to retrive the actions billing for an user", "user", *gitHubUser, "err", err)
			return
		}

		totalMinutesUsedActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.TotalMinutesUsed))
		includedMinutesUsedActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.IncludedMinutes))
		totalPaidMinutesActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.TotalPaidMinutesUsed))
		totalMinutesUsedUbuntuActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Ubuntu))
		totalMinutesUsedMacOSActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.MacOS))
		totalMinutesUsedWindowsActions.WithLabelValues("", *gitHubUser).Set(float64(actionsBilling.MinutesUsedBreakdown.Windows))
	}
}
