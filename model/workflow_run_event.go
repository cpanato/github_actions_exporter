package model

import (
	"encoding/json"
	"io"

	"github.com/google/go-github/v66/github"
)

func WorkflowRunEventFromJSON(data io.Reader) *github.WorkflowRunEvent {
	decoder := json.NewDecoder(data)
	var event github.WorkflowRunEvent
	if err := decoder.Decode(&event); err != nil {
		return nil
	}
	return &event
}
