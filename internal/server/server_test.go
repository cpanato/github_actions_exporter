package server_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/fernride/github_actions_exporter/internal/server"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Server_MetricsRouteWithNoMetrics(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	srv := server.NewServer(logger, server.Opts{
		MetricsPath:          "/metrics",
		ListenAddressMetrics: ":8000",
		ListenAddressIngress: ":8001",
		WebhookPath:          "/webhook",
		GitHubToken:          "webhook_token",
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

	res, err = http.Get("http://localhost:8000")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, 404, res.StatusCode)

	res, err = http.Get("http://localhost:8001")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, 200, res.StatusCode)

	payload, err = io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.NotNil(t, payload)
}
