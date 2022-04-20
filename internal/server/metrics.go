package server

import "github.com/prometheus/client_golang/prometheus"

var (
	workflowJobHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_job_duration_seconds",
		Help:    "Time that a workflow job took to reach a given state.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "state", "runner_group"},
	)

	workflowJobStatusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_status_count",
		Help: "Count of the occurances of different workflow job states.",
	},
		[]string{"org", "repo", "status", "runner_group"},
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
	//Register metrics with prometheus
	prometheus.MustRegister(workflowJobHistogramVec)
	prometheus.MustRegister(workflowJobStatusCounter)
	prometheus.MustRegister(workflowRunHistogramVec)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedUbuntuActions)
	prometheus.MustRegister(totalMinutesUsedMacOSActions)
	prometheus.MustRegister(totalMinutesUsedWindowsActions)
}

type WorkflowJobObserver interface {
	ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64)
	CountWorkflowJobStatus(org, repo, status, runnerGroup string)
	ObserveWorkflowRunDuration(org, repo, workflow string, seconds float64)
}

var _ WorkflowJobObserver = (*JobObserver)(nil)

type JobObserver struct{}

func (o *JobObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64) {
	workflowJobHistogramVec.WithLabelValues(org, repo, state, runnerGroup).
		Observe(seconds)
}

func (o *JobObserver) CountWorkflowJobStatus(org, repo, status, runnerGroup string) {
	workflowJobStatusCounter.WithLabelValues(org, repo, status, runnerGroup).Inc()
}

func (o *JobObserver) ObserveWorkflowRunDuration(org, repo, workflow string, seconds float64) {
	workflowRunHistogramVec.WithLabelValues(org, repo, workflow).
		Observe(seconds)
}
