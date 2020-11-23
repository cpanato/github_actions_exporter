package main

import (
	"github.com/google/go-github/v32/github"
	"github.com/prometheus/client_golang/prometheus"
)

var histogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "workflow_execution_time_seconds",
	Help:    "Time that a workflow took to run.",
	Buckets: prometheus.ExponentialBuckets(1, 1.4, 30),
},
	[]string{"org", "repo", "workflow_name"},
)

func init() {
	//Register metrics with prometheus
	prometheus.MustRegister(histogramVec)
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
