package model

import (
	"encoding/json"
	"io"

	"github.com/google/go-github/v66/github"
)

// PingEventFromJSON decodes the incomming message to a github.PingEvent
func PingEventFromJSON(data io.Reader) *github.PingEvent {
	decoder := json.NewDecoder(data)
	var event github.PingEvent
	if err := decoder.Decode(&event); err != nil {
		return nil
	}

	return &event
}
