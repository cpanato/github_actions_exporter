package server

import (
	"context"

	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

var client *github.Client = nil

func getGithubClient(githubToken string) *github.Client {
	if client != nil {
		return client
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	return client
}
