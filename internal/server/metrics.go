package server

type WorkflowJobObserver interface {
	ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64)
	ObserveWorkflowRunDuration(org, repo, workflow string, seconds float64)
}

var _ WorkflowJobObserver = (*JobObserver)(nil)

type JobObserver struct{}

func (o *JobObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64) {
	workflowJobHistogramVec.WithLabelValues(org, repo, state, runnerGroup).
		Observe(seconds)
}

func (o *JobObserver) ObserveWorkflowRunDuration(org, repo, workflow string, seconds float64) {
	workflowRunHistogramVec.WithLabelValues(org, repo, workflow).
		Observe(seconds)
}
