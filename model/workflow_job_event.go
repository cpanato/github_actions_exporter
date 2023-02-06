package model

import (
	"encoding/json"
	"github.com/google/go-github/v47/github"
)

type WorkflowJobEvent struct {
	WorkflowJob *github.WorkflowJob `json:"workflow_job,omitempty"`

	Action *string `json:"action,omitempty"`

	// The following fields are only populated by Webhook events.

	// Org is not nil when the webhook is configured for an organization or the event
	// occurs from activity in a repository owned by an organization.
	Org          *github.Organization `json:"organization,omitempty"`
	Repo         *github.Repository   `json:"repository,omitempty"`
	Sender       *github.User         `json:"sender,omitempty"`
	Installation *github.Installation `json:"installation,omitempty"`

	// Not present in google/go-github in v50.0.0 yet
	Deployment *github.Deployment `json:"deployment,omitempty"`
}

func WorkflowJobEventFromJSON(jsonString []byte) (*WorkflowJobEvent, error) {
	var event WorkflowJobEvent
	err := json.Unmarshal(jsonString, &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}
