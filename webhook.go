package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/go-kit/kit/log/level"

	"github.com/cpanato/github_actions_exporter/model"
)

// handleGHWebHook responds to POST /gh_event, when receive a event from GitHub.
func (c *GHActionExporter) handleGHWebHook(w http.ResponseWriter, r *http.Request) {
	buf, _ := ioutil.ReadAll(r.Body)

	receivedHash := strings.SplitN(r.Header.Get("X-Hub-Signature"), "=", 2)
	if receivedHash[0] != "sha1" {
		level.Error(c.Logger).Log("msg", "invalid webhook hash signature: SHA1")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := validateSignature(receivedHash, buf)
	if err != nil {
		level.Error(c.Logger).Log("msg", "invalid token", "err", err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "ping":
		pingEvent := model.PingEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		if pingEvent == nil {
			level.Info(c.Logger).Log("msg", "ping event", "hookID", pingEvent.GetHookID())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status": "honk"}`))
		return
	case "check_run":
		event := model.CheckRunEventFromJSON(ioutil.NopCloser(bytes.NewBuffer(buf)))
		level.Info(c.Logger).Log("msg", "got check_run event", "org", event.GetRepo().GetOwner().GetLogin(), "repo", event.GetRepo().GetName(), "workflowName", event.GetCheckRun().GetName())
		go CollectWorkflowRun(event)
	default:
		level.Info(c.Logger).Log("msg", "not implemented")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
}

// validateSignature validate the incoming github event.
func validateSignature(receivedHash []string, bodyBuffer []byte) error {
	hash := hmac.New(sha1.New, []byte(*gitHubToken))
	if _, err := hash.Write(bodyBuffer); err != nil {
		msg := fmt.Sprintf("Cannot compute the HMAC for request: %s\n", err)
		return errors.New(msg)
	}

	expectedHash := hex.EncodeToString(hash.Sum(nil))
	if receivedHash[1] != expectedHash {
		msg := fmt.Sprintf("Expected Hash does not match the received hash: %s\n", expectedHash)
		return errors.New(msg)
	}

	return nil
}
