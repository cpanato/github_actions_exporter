package server_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Server_MetricsRoute(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	srv := server.NewServer(logger, server.ServerOpts{
		MetricsPath:    "/metrics",
		ListenAddress:  ":8000",
		WebhookPath:    "/webhook",
		GitHubToken:    "webhook_token",
		GitHubUser:     "user",
		GitHubOrg:      "org",
		GitHubAPIToken: "api_token",
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

	payload, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	assert.NotNil(t, payload)
}
