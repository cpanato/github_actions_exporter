package model

import (
	"encoding/json"
	"io"

	"github.com/google/go-github/v66/github"
)

// CheckRunEventFromJSON decodes the incomming message to a github.CheckRunEvent
func CheckRunEventFromJSON(data io.Reader) *github.CheckRunEvent {
	decoder := json.NewDecoder(data)
	var event github.CheckRunEvent
	if err := decoder.Decode(&event); err != nil {
		return nil
	}

	return &event
}
