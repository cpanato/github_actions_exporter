package server_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/log"
	"github.com/google/go-github/v47/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Server_MetricsRouteWithNoMetrics(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	srv := server.NewServer(logger, server.Opts{
		MetricsPath:   "/metrics",
		ListenAddress: ":8000",
		WebhookPath:   "/webhook",
		GitHubToken:   "webhook_token",
	})

	t.Cleanup(func() {
		err := srv.Shutdown(context.Background())
		require.NoError(t, err)
	})

	go func() {
		t.Log("start server")
		err := srv.Serve(context.Background())
		require.NoError(t, err)
		t.Log("server shutdown")
	}()

	res, err := http.Get("http://localhost:8000/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, 200, res.StatusCode)

	payload, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.NotNil(t, payload)
}

func Test_Server_MetricsRouteAfterWorkflowJob(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	srv := server.NewServer(logger, server.Opts{
		MetricsPath:   "/metrics",
		ListenAddress: ":8000",
		WebhookPath:   "/webhook",
		GitHubToken:   webhookSecret,
	})

	t.Cleanup(func() {
		err := srv.Shutdown(context.Background())
		require.NoError(t, err)
	})

	go func() {
		t.Log("start server")
		err := srv.Serve(context.Background())
		require.NoError(t, err)
		t.Log("server shutdown")
	}()

	repo := "some-repo"
	org := "someone"
	expectedDuration := 10.0
	jobStartedAt := time.Unix(1650308740, 0)
	completedAt := jobStartedAt.Add(time.Duration(expectedDuration) * time.Second)
	runnerGroupName := "runner-group"

	event := github.WorkflowJobEvent{
		Action: github.String("completed"),
		Repo: &github.Repository{
			Name: &repo,
			Owner: &github.User{
				Login: &org,
			},
		},
		WorkflowJob: &github.WorkflowJob{
			Status:          github.String("completed"),
			Conclusion:      github.String("success"),
			StartedAt:       &github.Timestamp{Time: jobStartedAt},
			CompletedAt:     &github.Timestamp{Time: completedAt},
			RunnerGroupName: &runnerGroupName,
		},
	}
	req := testWebhookRequest(t, "http://localhost:8000/webhook", "workflow_job", event)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 202, res.StatusCode)

	time.Sleep(2 * time.Second)

	metricsRes, err := http.Get("http://localhost:8000/metrics")
	require.NoError(t, err)
	defer metricsRes.Body.Close()

	assert.Equal(t, 200, metricsRes.StatusCode)

	payload, err := io.ReadAll(metricsRes.Body)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `workflow_job_duration_seconds_bucket{org="someone",repo="some-repo",runner_group="runner-group",state="in_progress",le="10.541350399999995"} 1`)
	assert.Contains(t, string(payload), `workflow_job_duration_seconds_total{conclusion="success",org="someone",repo="some-repo",runner_group="runner-group",status="completed"} 10`)
}
