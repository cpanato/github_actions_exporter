package server

import "github.com/prometheus/client_golang/prometheus"

var (
	workflowJobHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_job_duration_seconds",
		Help:    "Time that a workflow job took to reach a given state.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "runner_group"},
	)

	workflowJobQueueTimeHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "workflow_job_queue_seconds",
		Help: "Time that a workflow job spent in a queued state.",
		// The buckets have been selected with the following assumptions:
		// 1. 10min is the GitHub SLO for larger runners, so we want to measure this accurately by having a 10min bucket.
		// 2. 5min is the GitHub SLO for hosted runners, so for the same reason we have a 5min bucket.
		// 3. In case of a longer queue time we have some buckets to capture it, but we don't need as much accuracy.
		// 4. In normal circumstances, queue times for hosted runners are often < 10s,
		// so we have more accuracy at lower queue times to measure it.
		// 5. Buckets are added between the thresholds to ensure the margin of error is lower.
		Buckets: []float64{2, 4, 6, 8, 10, 20, 30, 40, 50, 60, 120, 180, 240, 300, 360, 420, 480, 540, 600, 900, 1200},
	},
		[]string{"org", "repo", "runner_group"},
	)

	workflowJobDurationCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_duration_seconds_total",
		Help: "The total duration of jobs.",
	},
		[]string{"org", "repo", "status", "conclusion", "runner_group"},
	)

	workflowJobStatusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_status_count",
		Help: "Count of workflow job events.",
	},
		[]string{"org", "repo", "status", "conclusion", "runner_group"},
	)

	workflowRunHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_execution_time_seconds",
		Help:    "Time that a workflow took to run.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "workflow_name"},
	)

	workflowRunStatusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_status_count",
		Help: "Count of the occurrences of different workflow states.",
	},
		[]string{"org", "repo", "status", "conclusion", "workflow_name"},
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
	prometheus.MustRegister(workflowJobQueueTimeHistogram)
	prometheus.MustRegister(workflowJobStatusCounter)
	prometheus.MustRegister(workflowJobDurationCounter)
	prometheus.MustRegister(workflowRunHistogramVec)
	prometheus.MustRegister(workflowRunStatusCounter)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedUbuntuActions)
	prometheus.MustRegister(totalMinutesUsedMacOSActions)
	prometheus.MustRegister(totalMinutesUsedWindowsActions)
}

type WorkflowObserver interface {
	ObserveWorkflowJobDuration(org, repo, runnerGroup string, seconds float64)
	ObserveWorkflowJobQueueTime(org string, repo string, runnerGroup string, seconds float64)
	CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup string)
	CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup string, seconds float64)
	ObserveWorkflowRunDuration(org, repo, workflow string, seconds float64)
	CountWorkflowRunStatus(org, repo, status, conclusion, workflow string)
}

var _ WorkflowObserver = (*PrometheusObserver)(nil)

type PrometheusObserver struct{}

func (o *PrometheusObserver) ObserveWorkflowJobQueueTime(org string, repo string, runnerGroup string, seconds float64) {
	workflowJobQueueTimeHistogram.WithLabelValues(org, repo, runnerGroup).Observe(seconds)
}

func (o *PrometheusObserver) ObserveWorkflowJobDuration(org, repo, runnerGroup string, seconds float64) {
	workflowJobHistogramVec.WithLabelValues(org, repo, runnerGroup).Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup string) {
	workflowJobStatusCounter.WithLabelValues(org, repo, status, conclusion, runnerGroup).Inc()
}

func (o *PrometheusObserver) CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup string, seconds float64) {
	workflowJobDurationCounter.WithLabelValues(org, repo, status, conclusion, runnerGroup).Add(seconds)
}

func (o *PrometheusObserver) ObserveWorkflowRunDuration(org, repo, workflowName string, seconds float64) {
	workflowRunHistogramVec.WithLabelValues(org, repo, workflowName).
		Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowRunStatus(org, repo, status, conclusion, workflowName string) {
	workflowRunStatusCounter.WithLabelValues(org, repo, status, conclusion, workflowName).Inc()
}
