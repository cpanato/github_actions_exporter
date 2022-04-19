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

func Test_GHActionExporter_HandleGHWebHook_RejectsInvalidSignature(t *testing.T) {

	// Given
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)),
		Opts: server.ServerOpts{
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
	observer := NewTestJobObserver(t)
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
		JobObserver: observer,
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
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
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

func Test_GHActionExporter_HandleGHWebHook_CheckRun(t *testing.T) {

	// Given
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
	}
	req := testWebhookRequest(t, "/anything", "check_run", github.CheckRun{})

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobQueuedEvent(t *testing.T) {

	// Given
	observer := &testJobObserver{}
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
		JobObserver: observer,
	}
	event := github.WorkflowJobEvent{
		Action: github.String("queued"),
		Repo: &github.Repository{
			Name: github.String("some-repo"),
			Owner: &github.User{
				Login: github.String("someone"),
			},
		},
	}
	req := testWebhookRequest(t, "/anything", "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	observer.assertNoObservation(1 * time.Second)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobInProgressEvent(t *testing.T) {

	// Given
	observer := NewTestJobObserver(t)
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
		JobObserver: observer,
	}

	repo := "some-repo"
	org := "someone"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"

	event := github.WorkflowJobEvent{
		Action: github.String("in_progress"),
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
	observer.assetObservation(jobObservation{
		org:         org,
		repo:        repo,
		state:       "queued",
		runnerGroup: runnerGroupName,
		seconds:     expectedDuration,
	}, 50*time.Millisecond)
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

type jobObservation struct {
	org, repo, state, runnerGroup string
	seconds                       float64
}

var _ server.WorkflowJobObserver = (*testJobObserver)(nil)

type testJobObserver struct {
	t        *testing.T
	observed chan jobObservation
}

func NewTestJobObserver(t *testing.T) *testJobObserver {
	return &testJobObserver{
		t:        t,
		observed: make(chan jobObservation, 1),
	}
}

func (o *testJobObserver) ObserveWorkflowJobDuration(org, repo, state, runnerGroup string, seconds float64) {
	o.observed <- jobObservation{
		org:         org,
		repo:        repo,
		state:       state,
		runnerGroup: runnerGroup,
		seconds:     seconds,
	}
}

func (o *testJobObserver) assertNoObservation(timeout time.Duration) {
	select {
	case <-time.After(timeout):
	case <-o.observed:
		o.t.Fatal("expected no observation but an observation occurred")
	}
}

func (o *testJobObserver) assetObservation(expected jobObservation, timeout time.Duration) {
	select {
	case <-time.After(timeout):
		o.t.Fatal("expected no observation but none occurred")
	case observed := <-o.observed:
		assert.Equal(o.t, expected, observed)
	}
}
