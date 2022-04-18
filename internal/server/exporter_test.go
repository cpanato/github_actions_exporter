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
