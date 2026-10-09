package model_test

import (
	"io"
	"strings"
	"testing"

	"github.com/cpanato/github_actions_exporter/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PingEventFromJSON(t *testing.T) {
	event := model.PingEventFromJSON(strings.NewReader(`{"zen": "Keep it logically awesome.", "hook_id": 42}`))

	require.NotNil(t, event)
	assert.Equal(t, int64(42), event.GetHookID())
	assert.Equal(t, "Keep it logically awesome.", event.GetZen())
}

func Test_CheckRunEventFromJSON(t *testing.T) {
	event := model.CheckRunEventFromJSON(strings.NewReader(`{
		"action": "completed",
		"check_run": {"name": "build", "status": "completed", "conclusion": "success"},
		"repository": {"name": "repo", "owner": {"login": "org"}}
	}`))

	require.NotNil(t, event)
	assert.Equal(t, "completed", event.GetAction())
	assert.Equal(t, "build", event.GetCheckRun().GetName())
	assert.Equal(t, "success", event.GetCheckRun().GetConclusion())
	assert.Equal(t, "repo", event.GetRepo().GetName())
	assert.Equal(t, "org", event.GetRepo().GetOwner().GetLogin())
}

func Test_WorkflowJobEventFromJSON(t *testing.T) {
	event := model.WorkflowJobEventFromJSON(strings.NewReader(`{
		"action": "completed",
		"workflow_job": {
			"id": 1,
			"run_id": 2,
			"name": "Test",
			"head_branch": "main",
			"runner_group_name": "default",
			"workflow_name": "CI",
			"status": "completed",
			"conclusion": "success",
			"started_at": "2026-10-09T10:00:00Z",
			"completed_at": "2026-10-09T10:05:00Z"
		},
		"repository": {"name": "repo", "owner": {"login": "org"}}
	}`))

	require.NotNil(t, event)
	assert.Equal(t, "completed", event.GetAction())
	assert.Equal(t, "org", event.GetRepo().GetOwner().GetLogin())
	assert.Equal(t, "repo", event.GetRepo().GetName())

	job := event.GetWorkflowJob()
	assert.Equal(t, "Test", job.GetName())
	assert.Equal(t, "main", job.GetHeadBranch())
	assert.Equal(t, "default", job.GetRunnerGroupName())
	assert.Equal(t, "CI", job.GetWorkflowName())
	assert.Equal(t, int64(2), job.GetRunID())
	assert.Equal(t, "success", job.GetConclusion())
	assert.Equal(t, 5*60.0, job.GetCompletedAt().Sub(job.GetStartedAt().Time).Seconds())
}

func Test_WorkflowRunEventFromJSON(t *testing.T) {
	event := model.WorkflowRunEventFromJSON(strings.NewReader(`{
		"action": "completed",
		"workflow": {"name": "CI"},
		"workflow_run": {
			"head_branch": "main",
			"run_number": 7,
			"status": "completed",
			"conclusion": "failure",
			"run_started_at": "2026-10-09T10:00:00Z",
			"updated_at": "2026-10-09T10:10:00Z"
		},
		"repository": {"name": "repo", "owner": {"login": "org"}}
	}`))

	require.NotNil(t, event)
	assert.Equal(t, "completed", event.GetAction())
	assert.Equal(t, "CI", event.GetWorkflow().GetName())
	assert.Equal(t, "org", event.GetRepo().GetOwner().GetLogin())
	assert.Equal(t, "repo", event.GetRepo().GetName())

	run := event.GetWorkflowRun()
	assert.Equal(t, "main", run.GetHeadBranch())
	assert.Equal(t, 7, run.GetRunNumber())
	assert.Equal(t, "failure", run.GetConclusion())
}

// Invalid input must give a nil event, never a half decoded one.
func Test_EventsFromJSON_InvalidInput(t *testing.T) {
	decoders := map[string]func(io.Reader) any{
		"ping":         func(r io.Reader) any { return model.PingEventFromJSON(r) },
		"check_run":    func(r io.Reader) any { return model.CheckRunEventFromJSON(r) },
		"workflow_job": func(r io.Reader) any { return model.WorkflowJobEventFromJSON(r) },
		"workflow_run": func(r io.Reader) any { return model.WorkflowRunEventFromJSON(r) },
	}
	inputs := map[string]string{
		"empty body":     "",
		"not json":       "not json",
		"truncated json": `{"action": "comp`,
		"wrong type":     `{"action": 42, "hook_id": "not a number"}`,
		"json array":     `[]`,
	}

	for decoderName, decode := range decoders {
		for inputName, input := range inputs {
			t.Run(decoderName+"/"+inputName, func(t *testing.T) {
				// A typed nil pointer boxed in an interface is not == nil, so check through assert.
				assert.Nil(t, decode(strings.NewReader(input)))
			})
		}
	}
}
