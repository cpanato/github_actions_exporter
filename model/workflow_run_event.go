package model

import (
	"encoding/json"
	"github.com/google/go-github/v47/github"
)

func WorkflowRunEventFromJSON(jsonData []byte) (*github.WorkflowRunEvent, error) {
	var event github.WorkflowRunEvent
	err := json.Unmarshal(jsonData, &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}
