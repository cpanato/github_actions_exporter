package server_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1" // nolint: gosec
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pingPayload = `{"hook_id": 42}`

func newSignatureTestExporter() server.WorkflowMetricsExporter {
	return server.WorkflowMetricsExporter{
		Logger: slog.New(slog.DiscardHandler),
		Opts:   server.Opts{GitHubToken: webhookSecret},
	}
}

func hmacHex(newHash func() hash.Hash, payload []byte) string {
	h := hmac.New(newHash, []byte(webhookSecret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(payload []byte) string {
	return hmacHex(sha256.New, payload)
}

func sha1Hex(payload []byte) string {
	return hmacHex(sha1.New, payload)
}

func pingRequest(headers map[string]string, payload string) *http.Request {
	req := httptest.NewRequest("POST", "/anything", bytes.NewReader([]byte(payload)))
	req.Header.Set("X-GitHub-Event", "ping")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func Test_HandleGHWebHook_AcceptsValidSHA256Signature(t *testing.T) {
	subject := newSignatureTestExporter()
	req := pingRequest(map[string]string{
		"X-Hub-Signature-256": "sha256=" + sha256Hex([]byte(pingPayload)),
	}, pingPayload)

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
}

func Test_HandleGHWebHook_SHA256SignatureTakesPrecedenceOverSHA1(t *testing.T) {
	subject := newSignatureTestExporter()
	// A valid legacy signature must not rescue an invalid SHA-256 one.
	req := pingRequest(map[string]string{
		"X-Hub-Signature-256": "sha256=" + sha256Hex([]byte("another payload")),
		"X-Hub-Signature":     "sha1=" + sha1Hex([]byte(pingPayload)),
	}, pingPayload)

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusForbidden, res.Result().StatusCode)
}

func Test_HandleGHWebHook_FallsBackToSHA1Signature(t *testing.T) {
	subject := newSignatureTestExporter()
	req := pingRequest(map[string]string{
		"X-Hub-Signature": "sha1=" + sha1Hex([]byte(pingPayload)),
	}, pingPayload)

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusAccepted, res.Result().StatusCode)
}

func Test_HandleGHWebHook_RejectsWrongSecret(t *testing.T) {
	subject := newSignatureTestExporter()
	subject.Opts.GitHubToken = "another-secret"
	req := pingRequest(map[string]string{
		"X-Hub-Signature-256": "sha256=" + sha256Hex([]byte(pingPayload)),
	}, pingPayload)

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusForbidden, res.Result().StatusCode)
}

func Test_HandleGHWebHook_RejectsMalformedSignatures(t *testing.T) {
	validSHA256 := sha256Hex([]byte(pingPayload))
	validSHA1 := sha1Hex([]byte(pingPayload))

	tests := map[string]map[string]string{
		"no signature header":               {},
		"sha1 without digest":               {"X-Hub-Signature": "sha1"},
		"sha1 empty digest":                 {"X-Hub-Signature": "sha1="},
		"sha1 digest not hex":               {"X-Hub-Signature": "sha1=not-hex"},
		"sha256 algorithm in sha1 header":   {"X-Hub-Signature": "sha256=" + validSHA256},
		"sha1 algorithm in sha256 header":   {"X-Hub-Signature-256": "sha1=" + validSHA1},
		"sha256 without digest":             {"X-Hub-Signature-256": "sha256"},
		"sha256 digest not hex":             {"X-Hub-Signature-256": "sha256=zz"},
		"sha256 truncated digest":           {"X-Hub-Signature-256": "sha256=" + validSHA256[:10]},
		"digest without algorithm":          {"X-Hub-Signature-256": validSHA256},
		"uppercase algorithm not supported": {"X-Hub-Signature-256": "SHA256=" + validSHA256},
	}

	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			subject := newSignatureTestExporter()
			req := pingRequest(headers, pingPayload)

			res := httptest.NewRecorder()
			require.NotPanics(t, func() { subject.HandleGHWebHook(res, req) })

			assert.Equal(t, http.StatusForbidden, res.Result().StatusCode)
		})
	}
}

// zeroReader endlessly returns zero bytes.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func Test_HandleGHWebHook_RejectsTooLargeBody(t *testing.T) {
	subject := newSignatureTestExporter()
	const limit = 25 << 20
	req := httptest.NewRequest("POST", "/anything", io.LimitReader(zeroReader{}, limit+1))

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, res.Result().StatusCode)
}

func Test_HandleGHWebHook_AcceptsBodyAtTheLimit(t *testing.T) {
	subject := newSignatureTestExporter()
	const limit = 25 << 20
	req := httptest.NewRequest("POST", "/anything", io.LimitReader(zeroReader{}, limit))

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	// The body is read completely and only fails the signature check.
	assert.Equal(t, http.StatusForbidden, res.Result().StatusCode)
}

func Test_HandleGHWebHook_PingWithUndecodableBody(t *testing.T) {
	subject := newSignatureTestExporter()
	payload := "not json"
	req := pingRequest(map[string]string{
		"X-Hub-Signature-256": "sha256=" + sha256Hex([]byte(payload)),
	}, payload)

	res := httptest.NewRecorder()
	subject.HandleGHWebHook(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
}
