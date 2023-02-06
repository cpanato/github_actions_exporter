package server_test

import (
	"context"
	"github.com/cpanato/github_actions_exporter/model"
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
	startServer(t)

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
			ID:              github.Int64(62352),
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
	assert.Contains(t, string(payload), `workflow_job_duration_seconds_bucket{org="someone",repo="some-repo",runner_group="runner-group",le="10.541350399999995"} 1`)
	assert.Contains(t, string(payload), `workflow_job_duration_seconds_total{conclusion="success",org="someone",repo="some-repo",runner_group="runner-group",status="completed"} 10`)
}

// This handles a case where a job requires an approval to deploy to an environment.
// The events received are:
// 1. Job is scheduled
// 2. Event with `status: queued`
// 3. Event with `status: waiting`
// 4. Job manually approved
// 5. Another event with `status: queued`
// The event under 5. contains a `deployment` object with an `updated_at` time which we use to calculate the queue time
// to avoid adding the time to approve the deployment to the runner queue time.
func Test_Server_QueueTimeWithDeployment(t *testing.T) {
	startServer(t)

	expectedQueueTime := 9.0
	jobQueuedStartedAt := time.Unix(1650308740, 0)
	deploymentUpdatedAt := time.Unix(1650309923, 0)
	jobInProgressStartedAt := deploymentUpdatedAt.Add(time.Duration(expectedQueueTime) * time.Second)

	eventDeployment := github.Deployment{
		URL:         github.String("https://github.com/repos/org/repo-name/deployments/5535221"),
		ID:          github.Int64(5535221),
		Environment: github.String("test"),
		CreatedAt:   &github.Timestamp{Time: jobQueuedStartedAt},
		UpdatedAt:   &github.Timestamp{Time: deploymentUpdatedAt},
	}

	firstQueueEvent := eventWithDeployment(jobQueuedStartedAt, nil, "queued")
	waitingEvent := eventWithDeployment(jobQueuedStartedAt, &eventDeployment, "waiting")
	secondQueueEvent := eventWithDeployment(jobQueuedStartedAt, &eventDeployment, "queued")
	inProgressEvent := eventWithDeployment(jobInProgressStartedAt, &eventDeployment, "in_progress")

	sendEvent(t, firstQueueEvent)
	sendEvent(t, waitingEvent)
	sendEvent(t, secondQueueEvent)
	sendEvent(t, inProgressEvent)

	time.Sleep(2 * time.Second)

	metricsRes, err := http.Get("http://localhost:8000/metrics")
	require.NoError(t, err)
	defer metricsRes.Body.Close()

	assert.Equal(t, 200, metricsRes.StatusCode)

	payload, err := io.ReadAll(metricsRes.Body)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `workflow_job_queue_seconds_bucket{org="org-name",repo="repository-name",runner_group="runners",le="10"} 1`)
}

func sendEvent(t *testing.T, firstQueueEvent model.WorkflowJobEvent) {
	req := testWebhookRequest(t, "http://localhost:8000/webhook", "workflow_job", firstQueueEvent)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, 202, res.StatusCode)
	require.NoError(t, err)
}

func startServer(t *testing.T) {
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
}

func eventWithDeployment(jobQueuedStartedAt time.Time, eventDeployment *github.Deployment, status string) model.WorkflowJobEvent {
	return model.WorkflowJobEvent{
		Action: github.String(status),
		Repo: &github.Repository{
			Name: github.String("repository-name"),
			Owner: &github.User{
				Login: github.String("org-name"),
			},
		},
		WorkflowJob: &github.WorkflowJob{
			ID:              github.Int64(62352),
			Status:          github.String(status),
			StartedAt:       &github.Timestamp{Time: jobQueuedStartedAt},
			RunnerGroupName: github.String("runners"),
		},
		Deployment: eventDeployment,
	}
}
