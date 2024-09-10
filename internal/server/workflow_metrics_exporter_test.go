package server_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fernride/github_actions_exporter/internal/server"
	"github.com/go-kit/log"
	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	webhookSecret = "webhook-secret"
)

type readerThatErrors struct{}

func (readerThatErrors) Read(_ []byte) (n int, err error) {
	return 0, errors.New("test error")
}

func Test_WorkflowMetricsExporter_HandleGHWebHook_RejectsInvalidSignature(t *testing.T) {
	// Given
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
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
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
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

func Test_GHActionExporter_HandleGHWebHook_HandlesBodyReadError(t *testing.T) {
	// Given
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
	}
	req := httptest.NewRequest("POST", "/anything", readerThatErrors{})

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusInternalServerError, res.Result().StatusCode)
}

func Test_GHActionExporter_HandleGHWebHook_Ping(t *testing.T) {
	// Given
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
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
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	org := "org"
	repo := "repo"
	runnerGroupName := "runnerGroupName"
	action := "completed"
	status := "completed"
	conclusion := "success"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			Status:          &status,
			Conclusion:      &conclusion,
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
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
		org:          org,
		repo:         repo,
		status:       action,
		conclusion:   conclusion,
		runnerGroup:  runnerGroupName,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobInProgressEvent(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"
	action := "in_progress"
	status := "in_progress"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			HeadBranch: &branch,
			Status:     &status,
			StartedAt:  &github.Timestamp{Time: jobStartedAt},
			Steps: []*github.TaskStep{
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt},
				},
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt.Add(5 * time.Second)},
				},
			},
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowJobObservation(workflowJobObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		state:        "queued",
		runnerGroup:  runnerGroupName,
		seconds:      expectedDuration,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		runnerGroup:  runnerGroupName,
		status:       action,
		conclusion:   "",
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_WorkflowMetricsExporter_HandleGHWebHook_WorkflowJobInProgressEventWithNegativeDuration(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(-1 * time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"
	action := "in_progress"
	status := "in_progress"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			HeadBranch: &branch,
			Status:     &status,
			StartedAt:  &github.Timestamp{Time: jobStartedAt},
			Steps: []*github.TaskStep{
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt},
				},
				{
					StartedAt: &github.Timestamp{Time: stepStartedAt.Add(5 * time.Second)},
				},
			},
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowJobObservation(workflowJobObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		state:        "queued",
		runnerGroup:  runnerGroupName,
		workflowName: workflowName,
		jobName:      jobName,
		seconds:      0,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		runnerGroup:  runnerGroupName,
		status:       action,
		conclusion:   "",
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobCompletedEvent(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"
	expectedDuration := 5.0
	startedAt := time.Unix(1650308740, 0)
	completedAt := startedAt.Add(time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"
	action := "completed"
	status := "completed"
	conclusion := "success"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			HeadBranch:      &branch,
			StartedAt:       &github.Timestamp{Time: startedAt},
			CompletedAt:     &github.Timestamp{Time: completedAt},
			Status:          &status,
			Conclusion:      &conclusion,
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowJobObservation(workflowJobObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		state:        "in_progress",
		runnerGroup:  runnerGroupName,
		seconds:      expectedDuration,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		runnerGroup:  runnerGroupName,
		status:       status,
		conclusion:   conclusion,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
	observer.assertWorkflowJobDurationCount(workflowJobDurationCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		runnerGroup:  runnerGroupName,
		status:       status,
		conclusion:   conclusion,
		seconds:      expectedDuration,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobCompletedEvent_WithNoStartedAt(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"

	runnerGroupName := "runner-group"
	action := "completed"
	status := "completed"
	conclusion := "success"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			HeadBranch:      &branch,
			CompletedAt:     &github.Timestamp{Time: time.Unix(1650308740, 0)},
			Conclusion:      &conclusion,
			Status:          &status,
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		status:       status,
		conclusion:   conclusion,
		runnerGroup:  runnerGroupName,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobCompletedEvent_WithNoCompletedAt(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"
	runnerGroupName := "runner-group"
	action := "completed"
	status := "completed"
	conclusion := "success"
	workflowName := "Build and test"
	jobName := "Test"

	event := github.WorkflowJobEvent{
		Action: &action,
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			HeadBranch:      &branch,
			StartedAt:       &github.Timestamp{Time: time.Unix(1650308740, 0)},
			Conclusion:      &conclusion,
			Status:          &status,
			RunnerGroupName: &runnerGroupName,
			WorkflowName:    &workflowName,
			Name:            &jobName,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowJobStatusCount(workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		runnerGroup:  runnerGroupName,
		status:       status,
		conclusion:   conclusion,
		workflowName: workflowName,
		jobName:      jobName,
	}, 50*time.Millisecond)
}

func Test_WorkflowMetricsExporter_HandleGHWebHook_WorkflowRunCompleted(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	branch := "some-branch"
	workflowName := "myworkflow"
	expectedRunDuration := 5.0
	runStartTime := time.Unix(1650308740, 0)
	runUpdatedTime := runStartTime.Add(time.Duration(expectedRunDuration) * time.Second)
	status := "completed"
	conclusion := "success"

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
			HeadBranch:   &branch,
			Status:       &status,
			RunStartedAt: &github.Timestamp{Time: runStartTime},
			UpdatedAt:    &github.Timestamp{Time: runUpdatedTime},
			Conclusion:   &conclusion,
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_run", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertWorkflowRunObservation(workflowRunObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		workflowName: workflowName,
		conclusion:   conclusion,
		seconds:      expectedRunDuration,
	}, 50*time.Millisecond)
	observer.assertWorkflowRunStatusCount(workflowRunStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		workflowName: workflowName,
		status:       status,
		conclusion:   conclusion,
	}, 50*time.Millisecond)
}

func Test_WorkflowMetricsExporter_HandleGHWebHook_WorkflowRunEventOtherThanCompleted(t *testing.T) {
	// Given
	observer := NewTestPrometheusObserver(t)
	subject := server.WorkflowMetricsExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.Opts{
			GitHubToken: webhookSecret,
		},
		PrometheusObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	workflowName := "myworkflow"
	expectedRunDuration := 5.0
	runStartTime := time.Unix(1650308740, 0)
	runUpdatedTime := runStartTime.Add(time.Duration(expectedRunDuration) * time.Second)
	branch := "some-branch"

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
			HeadBranch:   &branch,
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
	org, repo, branch, state, runnerGroup, workflowName, jobName string
	seconds                                                      float64
}
type workflowJobStatusCount struct {
	org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string
}

type workflowJobDurationCount struct {
	org, repo, branch, status, conclusion, runnerGroup, workflowName, jobName string
	seconds                                                                   float64
}

type workflowRunObservation struct {
	org, repo, branch, workflowName, conclusion string
	seconds                                     float64
}

type workflowRunStatusCount struct {
	org, repo, branch, status, conclusion, workflowName string
}

var _ server.WorkflowObserver = (*TestPrometheusObserver)(nil)

type TestPrometheusObserver struct {
	t                           *testing.T
	workFlowJobDurationObserved chan workflowJobObservation
	workflowJobStatusCounted    chan workflowJobStatusCount
	workflowJobDurationCounted  chan workflowJobDurationCount
	workflowRunObserved         chan workflowRunObservation
	workflowRunStatusCounted    chan workflowRunStatusCount
}

func NewTestPrometheusObserver(t *testing.T) *TestPrometheusObserver {
	return &TestPrometheusObserver{
		t:                           t,
		workFlowJobDurationObserved: make(chan workflowJobObservation, 1),
		workflowJobStatusCounted:    make(chan workflowJobStatusCount, 1),
		workflowJobDurationCounted:  make(chan workflowJobDurationCount, 1),
		workflowRunObserved:         make(chan workflowRunObservation, 1),
		workflowRunStatusCounted:    make(chan workflowRunStatusCount, 1),
	}
}

func (o *TestPrometheusObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup, workflowName, jobName, branch string, seconds float64) {
	o.workFlowJobDurationObserved <- workflowJobObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		state:        state,
		runnerGroup:  runnerGroup,
		workflowName: workflowName,
		jobName:      jobName,
		seconds:      seconds,
	}
}

func (o *TestPrometheusObserver) CountWorkflowJobStatus(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string) {
	o.workflowJobStatusCounted <- workflowJobStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		status:       status,
		conclusion:   conclusion,
		runnerGroup:  runnerGroup,
		workflowName: workflowName,
		jobName:      jobName,
	}
}

func (o *TestPrometheusObserver) CountWorkflowJobDuration(org, repo, status, conclusion, runnerGroup, workflowName, jobName, branch string, seconds float64) {
	o.workflowJobDurationCounted <- workflowJobDurationCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		status:       status,
		conclusion:   conclusion,
		runnerGroup:  runnerGroup,
		workflowName: workflowName,
		jobName:      jobName,
		seconds:      seconds,
	}
}

func (o *TestPrometheusObserver) ObserveWorkflowRunDuration(org, repo, branch, workflowName, conclusion string, seconds float64) {
	o.workflowRunObserved <- workflowRunObservation{
		org:          org,
		repo:         repo,
		branch:       branch,
		workflowName: workflowName,
		seconds:      seconds,
		conclusion:   conclusion,
	}
}

func (o *TestPrometheusObserver) CountWorkflowRunStatus(org, repo, branch, status, conclusion, workflowName string) {
	o.workflowRunStatusCounted <- workflowRunStatusCount{
		org:          org,
		repo:         repo,
		branch:       branch,
		status:       status,
		conclusion:   conclusion,
		workflowName: workflowName,
	}
}

func (o *TestPrometheusObserver) assertNoWorkflowJobDurationObservation(timeout time.Duration) {
	select {
	case <-time.After(timeout):
	case <-o.workFlowJobDurationObserved:
		o.t.Fatal("expected no observation but an observation occurred")
	}
}

func (o *TestPrometheusObserver) assertWorkflowJobObservation(expected workflowJobObservation, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workFlowJobDurationObserved:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestPrometheusObserver) assertWorkflowJobStatusCount(expected workflowJobStatusCount, timeout time.Duration) { //nolint: unparam
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowJobStatusCounted:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestPrometheusObserver) assertWorkflowJobDurationCount(expected workflowJobDurationCount, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowJobDurationCounted:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestPrometheusObserver) assertWorkflowRunObservation(expected workflowRunObservation, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowRunObserved:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestPrometheusObserver) assertWorkflowRunStatusCount(expected workflowRunStatusCount, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected observation but none occurred")
	case observed := <-o.workflowRunStatusCounted:
		assert.Equal(o.t, expected, observed)
	}
}

func (o *TestPrometheusObserver) assertNoWorkflowRunStatusCount(timeout time.Duration) {
	select {
	case <-time.After(timeout):
	case <-o.workflowRunObserved:
		o.t.Fatal("expected no observation but an observation occurred")
	}
}
