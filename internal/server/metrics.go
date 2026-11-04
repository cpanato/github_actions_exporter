package server

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

var (
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
)

func init() {
	// Register metrics with prometheus
	prometheus.MustRegister(totalMinutesUsedActions)
	prometheus.MustRegister(includedMinutesUsedActions)
	prometheus.MustRegister(totalPaidMinutesActions)
	prometheus.MustRegister(totalMinutesUsedByHostTypeActions)
}

type WorkflowObserver interface {
	ObserveWorkflowJobDuration(org, repo, branch, state, runnerGroup, workflowName, jobName string, seconds float64)
	CountWorkflowJobStatus(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string)
	CountWorkflowJobDuration(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string, seconds float64)

	ObserveWorkflowRunDuration(org, repo, branch, workflow, conclusion string, seconds float64)
	CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflow string)
}

var _ WorkflowObserver = (*PrometheusObserver)(nil)

// PrometheusObserver records the workflow metrics in Prometheus.
type PrometheusObserver struct {
	omitBranch bool

	workflowJobHistogramVec    *prometheus.HistogramVec
	workflowJobDurationCounter *prometheus.CounterVec
	workflowJobStatusCounter   *prometheus.CounterVec
	workflowRunHistogramVec    *prometheus.HistogramVec
	workflowRunStatusCounter   *prometheus.CounterVec
}

// NewPrometheusObserver registers the workflow metrics with reg. When omitBranch is true,
// the metrics have no branch label, so all branches of a workflow share the same series.
// Registering a second observer with the same labels reuses the metrics of the first.
func NewPrometheusObserver(reg prometheus.Registerer, omitBranch bool) *PrometheusObserver {
	o := &PrometheusObserver{omitBranch: omitBranch}

	o.workflowJobHistogramVec = registerOrExisting(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_job_duration_seconds",
		Help:    "Time that a workflow job took to reach a given state.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		o.labelNames("state", "runner_group", "workflow_name", "job_name"),
	))

	o.workflowJobDurationCounter = registerOrExisting(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_duration_seconds_total",
		Help: "The total duration of jobs.",
	},
		o.labelNames("status", "conclusion", "runner_group", "workflow_name", "job_name"),
	))

	o.workflowJobStatusCounter = registerOrExisting(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_job_status_count",
		Help: "Count of workflow job events.",
	},
		o.labelNames("status", "conclusion", "runner_group", "workflow_name", "job_name"),
	))

	o.workflowRunHistogramVec = registerOrExisting(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_execution_time_seconds",
		Help:    "Time that a workflow took to run.",
		Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
	},
		o.labelNames("workflow_name", "conclusion"),
	))

	o.workflowRunStatusCounter = registerOrExisting(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_status_count",
		Help: "Count of the occurrences of different workflow states.",
	},
		o.labelNames("status", "conclusion", "workflow_name"),
	))

	return o
}

func registerOrExisting[T prometheus.Collector](reg prometheus.Registerer, c T) T {
	if err := reg.Register(c); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			if existing, ok := are.ExistingCollector.(T); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// labelNames returns the org, repo and (unless omitted) branch labels followed by names.
func (o *PrometheusObserver) labelNames(names ...string) []string {
	if o.omitBranch {
		return append([]string{"org", "repo"}, names...)
	}
	return append([]string{"org", "repo", "branch"}, names...)
}

// labelValues returns the values matching labelNames.
func (o *PrometheusObserver) labelValues(org, repo, branch string, values ...string) []string {
	if o.omitBranch {
		return append([]string{org, repo}, values...)
	}
	return append([]string{org, repo, branch}, values...)
}

func (o *PrometheusObserver) ObserveWorkflowJobDuration(org, repo, branch, state, runnerGroup, workflowName, jobName string, seconds float64) {
	o.workflowJobHistogramVec.WithLabelValues(o.labelValues(org, repo, branch, state, runnerGroup, workflowName, jobName)...).
		Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowJobStatus(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string) {
	o.workflowJobStatusCounter.WithLabelValues(o.labelValues(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName)...).Inc()
}

func (o *PrometheusObserver) CountWorkflowJobDuration(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string, seconds float64) {
	o.workflowJobDurationCounter.WithLabelValues(o.labelValues(org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName)...).Add(seconds)
}

func (o *PrometheusObserver) ObserveWorkflowRunDuration(org, repo, branch, workflowName, conclusion string, seconds float64) {
	o.workflowRunHistogramVec.WithLabelValues(o.labelValues(org, repo, branch, workflowName, conclusion)...).
		Observe(seconds)
}

func (o *PrometheusObserver) CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflowName string) {
	o.workflowRunStatusCounter.WithLabelValues(o.labelValues(org, repo, branch, status, conclusion, workflowName)...).Inc()
}
