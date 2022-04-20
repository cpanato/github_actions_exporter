package server_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/kit/log"
	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	webhookSecret = "webhook-secret"
)

func Test_WorkflowExporter_HandleGHWebHook_RejectsInvalidSignature(t *testing.T) {
	// Given
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	req, err := http.NewRequest("POST", "/anything", bytes.NewReader(nil))
	require.NoError(t, err)
	req.Header.Add("X-Hub-Signature", "sha1=incorrect")

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusForbidden, res.Result().StatusCode)
}

func Test_GHActionExporter_HandleGHWebHook_ValidatesValidSignature(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	req, err := http.NewRequest("POST", "/anything", bytes.NewReader(nil))
	require.NoError(t, err)
	addValidSignatureHeader(t, req, nil)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusNotImplemented, res.Result().StatusCode)
}

func Test_GHActionExporter_HandleGHWebHook_Ping(t *testing.T) {
	// Given
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}
	req := testWebhookRequest(t, "/anything", "ping", github.PingEvent{})

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	assert.Equal(t, `{"status": "honk"}`, res.Body.String())
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobQueuedEvent(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	org := "org"
	repo := "repo"
	runnerGroupName := "runnerGroupName"
	action := "queued"
	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertNoWorkflowJobDurationObservation(1 * time.Second)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroupName,
		status:      action,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobInProgressEvent(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"
	action := "in_progress"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			StartedAt: &github.Timestamp{Time: jobStartedAt},
			Steps: []*github.TaskStep{
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt},
				},
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt.Add(5 * time.Second)},
				},
			},
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assetWorkflowJobObservation(workflowJobObservation{
		org:         org,
		repo:        repo,
		state:       "queued",
		runnerGroup: runnerGroupName,
		seconds:     expectedDuration,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroupName,
		status:      action,
	}, 50*time.Millisecond)
}

func Test_WorkflowExporter_HandleGHWebHook_WorkflowJobInProgressEventWithNegativeDuration(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(-1 * time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"
	action := "in_progress"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			StartedAt: &github.Timestamp{Time: jobStartedAt},
			Steps: []*github.TaskStep{
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt},
				},
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt.Add(5 * time.Second)},
				},
			},
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assetWorkflowJobObservation(workflowJobObservation{
		org:         org,
		repo:        repo,
		state:       "queued",
		runnerGroup: runnerGroupName,
		seconds:     0,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroupName,
		status:      action,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobCompletedEvent(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	expectedStepTime := 5.0

	firstStepStartedAt := time.Unix(1650308740, 0)
	lastStepStartedAt := firstStepStartedAt.Add(time.Duration(expectedStepTime) * time.Second)
	lastStepFinishedAt := lastStepStartedAt.Add(time.Duration(expectedStepTime) * time.Second)
	runnerGroupName := "runner-group"
	state := "success"

	event := github.WorkflowJobEvent{
		Action: github.String("completed"),
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			StartedAt:  nil,
			Conclusion: &state,
			Steps: []*github.TaskStep{
				{
					Number:    github.Int64(1),
					StartedAt: &github.Timestamp{Time: firstStepStartedAt},
				},
				{
					Number:      github.Int64(2),
					StartedAt:   &github.Timestamp{Time: lastStepStartedAt},
					CompletedAt: &github.Timestamp{Time: lastStepFinishedAt},
				},
			},
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assetWorkflowJobObservation(workflowJobObservation{
		org:         org,
		repo:        repo,
		state:       "in_progress",
		runnerGroup: runnerGroupName,
		seconds:     expectedStepTime * 2,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroupName,
		status:      state,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobCompletedEventWithSkippedConclusion(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	runnerGroupName := "runner-group"
	state := "skipped"

	event := github.WorkflowJobEvent{
		Action: github.String("completed"),
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			StartedAt:       nil,
			Conclusion:      &state,
			Steps:           []*github.TaskStep{},
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertNoWorkflowJobDurationObservation(1 * time.Second)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroupName,
		status:      state,
	}, 50*time.Millisecond)
}

func Test_WorkflowExporter_HandleGHWebHook_WorkflowRunCompleted(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	workflowName := "myworkflow"
	expectedRunDuration := 5.0
	runStartTime := time.Unix(1650308740, 0)
	runUpdatedTime := runStartTime.Add(time.Duration(expectedRunDuration) * time.Second)
	status := "completed"
	event := github.WorkflowRunEvent{
		Action: github.String("completed"),
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		Workflow: &github.Workflow{
			Name: &workflowName,
		},
		WorkflowRun: &github.WorkflowRun{
			Status:       &status,
			RunStartedAt: &github.Timestamp{Time: runStartTime},
			UpdatedAt:    &github.Timestamp{Time: runUpdatedTime},
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_run", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assetWorkflowRunObservation(workflowRunObservation{
		org:          org,
		repo:         repo,
		workflowName: workflowName,
		seconds:      expectedRunDuration,
	}, 50*time.Millisecond)
	observer.assertWorkflowRunStatusCount(workflowRunStatusCount{
		org:          org,
		repo:         repo,
		workflowName: workflowName,
		status:       status,
	}, 50*time.Millisecond)
}

func Test_WorkflowExporter_HandleGHWebHook_WorkflowRunEventOtherThanCompleted(t *testing.T) {
	// Given
	observer := NewTestJobObserver(t)
	subject := server.WorkflowExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		JobObserver:            observer,
		BillingMetricsExporter: &server.BillingMetricsExporter{},
	}

	repo := "some-repo"
	org := "someone"
	workflowName := "myworkflow"
	expectedRunDuration := 5.0
	runStartTime := time.Unix(1650308740, 0)
	runUpdatedTime := runStartTime.Add(time.Duration(expectedRunDuration) * time.Second)
	event := github.WorkflowRunEvent{
		Action: github.String("not_a_completed_action"),
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		Workflow: &github.Workflow{
			Name: &workflowName,
		},
		WorkflowRun: &github.WorkflowRun{
			Status:       github.String("completed"),
			RunStartedAt: &github.Timestamp{Time: runStartTime},
			UpdatedAt:    &github.Timestamp{Time: runUpdatedTime},
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_run", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertNoWorkflowJobDurationObservation(1 * time.Second)
	observer.assertNoWorkflowRunStatusCount(1 * time.Second)
}

func testWebhookRequest(t *testing.T, url, event string, payload interface{}) *http.Request {
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	require.NoError(t, err)

	addValidSignatureHeader(t, req, b)
	req.Header.Add("X-GitHub-Event", event)
	return req
}

func addValidSignatureHeader(t *testing.T, req *http.Request, payload []byte) {
	h := hmac.New(sha1.New, []byte(webhookSecret))
	_, err := h.Write(payload)
	require.NoError(t, err)

	req.Header.Add("X-Hub-Signature", fmt.Sprintf("sha1=%s", hex.EncodeToString(h.Sum(nil))))
}

type workflowJobObservation struct {
	org, repo, state, runnerGroup string
	seconds                       float64
}
type workflowJobStatusCount struct {
	org, repo, runnerGroup, status string
}

type workflowRunObservation struct {
	org, repo, workflowName string
	seconds                 float64
}

type workflowRunStatusCount struct {
	org, repo, workflowName, status string
}

var _ server.WorkflowJobObserver = (*TestJobObserver)(nil)

type TestJobObserver struct {
	t                           *testing.T
	workFlowJobDurationObserved chan workflowJobObservation
	workflowJobStatusCounted    chan workflowJobStatusCount
	workflowRunObserved         chan workflowRunObservation
	workflowRunStatusCounted    chan workflowRunStatusCount
}

func NewTestJobObserver(t *testing.T) *TestJobObserver {
	return &TestJobObserver{
		t:                           t,
		workFlowJobDurationObserved: make(chan workflowJobObservation, 1),
		workflowJobStatusCounted:    make(chan workflowJobStatusCount, 1),
		workflowRunObserved:         make(chan workflowRunObservation, 1),
		workflowRunStatusCounted:    make(chan workflowRunStatusCount, 1),
	}
}

func (o *TestJobObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64) {
	o.workFlowJobDurationObserved <- workflowJobObservation{
		org:         org,
		repo:        repo,
		state:       state,
		runnerGroup: runnerGroup,
		seconds:     seconds,
	}
}

func (o *TestJobObserver) CountWorkflowJobStatus(org, repo, status, runnerGroup string) {
	o.workflowJobStatusCounted <- workflowJobStatusCount{
		org:         org,
		repo:        repo,
		runnerGroup: runnerGroup,
		status:      status,
	}
}

func (o *TestJobObserver) ObserveWorkflowRunDuration(org, repo, workflowName string, seconds float64) {
	o.workflowRunObserved <- workflowRunObservation{
		org:          org,
		repo:         repo,
		workflowName: workflowName,
		seconds:      seconds,
	}
}

func (o *TestJobObserver) CountWorkflowRunStatus(org, repo, workflowName, status string) {
	o.workflowRunStatusCounted <- workflowRunStatusCount{
		org:          org,
		repo:         repo,
		workflowName: workflowName,
		status:       status,
	}
}

func (o *TestJobObserver) assertNoWorkflowJobDurationObservation(timeout time.Duration) {
	select {
	case <-time.After(timeout):
	case <-o.workFlowJobDurationObserved:
		o.t.Fatal("expected no observation but an observation occurred")
	}
}

func (o *TestJobObserver) assetWorkflowJobObservation(expected workflowJobObservation, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workFlowJobDurationObserved:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestJobObserver) assertWorkflowJobStatusCount(expected workflowJobStatusCount, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowJobStatusCounted:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestJobObserver) assetWorkflowRunObservation(expected workflowRunObservation, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowRunObserved:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestJobObserver) assertWorkflowRunStatusCount(expected workflowRunStatusCount, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowRunStatusCounted:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestJobObserver) assertNoWorkflowRunStatusCount(timeout time.Duration) {
	select {
	case <-time.After(timeout):
	case <-o.workflowRunObserved:
		o.t.Fatal("expected no observation but an observation occurred")
	}
}
