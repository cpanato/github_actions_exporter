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
	"strings"
	"testing"
	"time"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/kit/log"
	"github.com/google/go-github/v43/github"
	"github.com/prometheus/client_golang/prometheus/testutil"
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
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
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

	req := testValidRequest(t, "ping", github.PingEvent{})

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

	req := testValidRequest(t, "check_run", github.CheckRun{})

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobQueuedEvent(t *testing.T) {

	// Given
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
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
	req := testValidRequest(t, "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
	assert.Equal(t, 0, testutil.CollectAndCount(server.WorkflowHistogramVec))
}

func Test_GHActionExporter_HandleGHWebHook_WorkflowJobInProgressEvent(t *testing.T) {

	// Given
	subject := server.GHActionExporter{
		Logger: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		Opts: server.ServerOpts{
			GitHubToken: webhookSecret,
		},
	}

	repo := "some-repo"
	org := "someone"
	queuedDuration := 10
	jobStartedAt := time.Unix(1650308740, 0)
	stepStartedAt := jobStartedAt.Add(time.Duration(queuedDuration) * time.Second)
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
	req := testValidRequest(t, "workflow_job", event)

	// When
	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// Then
	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)

	expected := `# HELP workflow_job_seconds Time that a workflow job took to reach a given state.
	# TYPE workflow_job_seconds histogram
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="1"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="1.4"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="1.9599999999999997"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="2.7439999999999993"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="3.841599999999999"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="5.378239999999998"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="7.529535999999997"} 0
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="10.541350399999995"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="14.757890559999993"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="20.66104678399999"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="28.925465497599983"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="40.495651696639975"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="56.69391237529596"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="79.37147732541433"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="111.12006825558007"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="155.5680955578121"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="217.79533378093691"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="304.91346729331167"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="426.8788542106363"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="597.6303958948907"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="836.682554252847"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="1171.3555759539856"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="1639.8978063355798"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="2295.856928869812"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="3214.1997004177365"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="4499.87958058483"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="6299.831412818762"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="8819.763977946266"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="12347.669569124771"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="17286.73739677468"} 1
	workflow_job_seconds_bucket{org="some-repo",repo="someone",runner_group="runner-group",state="queued",le="+Inf"} 1
	workflow_job_seconds_sum{org="some-repo",repo="someone",runner_group="runner-group",state="queued"} 10
	workflow_job_seconds_count{org="some-repo",repo="someone",runner_group="runner-group",state="queued"} 1
	
`
	err := testutil.CollectAndCompare(server.WorkflowHistogramVec, strings.NewReader(expected))
	require.NoError(t, err)
}

func testValidRequest(t *testing.T, event string, payload interface{}) *http.Request {
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/anything", bytes.NewReader(b))
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
