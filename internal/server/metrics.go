package server

import "github.com/prometheus/client_golang/prometheus"

var (
	workflowJobHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_job_duration_seconds",
		Help:    "Time that a workflow job took to reach a given state.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "state", "runner_group", "workflow_name", "job_name", "branch"},
	)

	workflowJobDurationCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_duration_seconds_total",
		Help: "The total duration of jobs.",
	},
		[]string{"org", "repo", "status", "conclusion", "runner_group", "workflow_name", "job_name", "branch"},
	)

	workflowJobStatusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_status_count",
		Help: "Count of workflow job events.",
	},
		[]string{"org", "repo", "status", "conclusion", "runner_group", "workflow_name", "job_name", "branch"},
	)

	workflowRunHistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_execution_time_seconds",
		Help:    "Time that a workflow took to run.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		[]string{"org", "repo", "branch", "workflow_name", "conclusion"},
	)

	workflowRunStatusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_status_count",
		Help: "Count of the occurrences of different workflow states.",
	},
		[]string{"org", "repo", "branch", "status", "conclusion", "workflow_name"},
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

	totalMinutesUsedByHostTypeActions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "actions_total_minutes_used_by_host_minutes",
		Help: "Total minutes used for a specific host type for the GitHub Actions.",
	},
		[]string{"org", "user", "host_type"},
	)

	workflowQueueSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "workflow_queue_size",
		Help: "Number of queued workflow runs.",
	},
		[]string{},
	)
)

func init() {
	// Register metrics with prometheus
	prometheus.MustRegister(workflowJobHistogramVec)
	prometheus.MustRegister(workflowJobStatusCounter)
	prometheus.MustRegister(workflowJobDurationCounter)
	prometheus.MustRegister(workflowRunHistogramVec)
	prometheus.MustRegister(workflowRunStatusCounter)
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedByHostTypeActions)
	prometheus.MustRegister(workflowQueueSize)

}

type WorkflowObserver interface {
	ObserveWorkflowJobDuration(org, repo, state, runnerGroup, workflowName, jobName, branch string, seconds float64)
	CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string)
	CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string, seconds float64)

	ObserveWorkflowRunDuration(org, repo, branch, workflow, conclusion string, seconds float64)
	CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflow string)
}

var _ WorkflowObserver = (*PrometheusObserver)(nil)

type PrometheusObserver struct{}

func (o *PrometheusObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup, workflowName, jobName, branch string, seconds float64) {
	workflowJobHistogramVec.WithLabelValues(org, repo, state, runnerGroup, workflowName, jobName, branch).
		Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string) {
	workflowJobStatusCounter.WithLabelValues(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch).Inc()
}

func (o *PrometheusObserver) CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string, seconds float64) {
	workflowJobDurationCounter.WithLabelValues(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch).Add(seconds)
}

func (o *PrometheusObserver) ObserveWorkflowRunDuration(org, repo, branch, workflowName, conclusion string, seconds float64) {
	workflowRunHistogramVec.WithLabelValues(org, repo, branch, workflowName, conclusion).
		Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflowName string) {
	workflowRunStatusCounter.WithLabelValues(org, repo, branch, status, conclusion, workflowName).Inc()
}
