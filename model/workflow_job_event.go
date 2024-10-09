package model

import (
	"encoding/json"
	"io"

	"github.com/google/go-github/v66/github"
)

func WorkflowJobEventFromJSON(data io.Reader) *github.WorkflowJobEvent {
	decoder := json.NewDecoder(data)
	var event github.WorkflowJobEvent
	if err := decoder.Decode(&event); err != nil {
		return nil
	}
	return &event
}
