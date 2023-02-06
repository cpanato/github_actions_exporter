package model

import (
	"encoding/json"
	"github.com/google/go-github/v47/github"
)

// PingEventFromJSON decodes the incomming message to a github.PingEvent
func PingEventFromJSON(jsonData []byte) (*github.PingEvent, error) {
	var event github.PingEvent
	err := json.Unmarshal(jsonData, &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}
