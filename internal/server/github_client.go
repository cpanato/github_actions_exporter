package server

import (
	"context"
	"errors"
	"github.com/google/go-github/v47/github"
	"strconv"
)

const (
	pageSize = 100
)

type GitHubClient interface {
	GetOrganisationRunnerGroups(ctx context.Context, orgName string) ([]*github.RunnerGroup, error)
	GetEnterpriseRunners(ctx context.Context, enterpriseName string) ([]*github.Runner, error)
	GetGroupRunners(ctx context.Context, groupID int64, orgName string) ([]*github.Runner, error)
	GetActionsBillingOrg(ctx context.Context, org string) (*github.ActionBilling, error)
	GetActionsBillingUser(ctx context.Context, user string) (*github.ActionBilling, error)
}

type DefaultGitHubClient struct {
	Client *github.Client
	Opts   *Opts
}

func NewGitHubClient(opts *Opts, client *github.Client) *DefaultGitHubClient {
	return &DefaultGitHubClient{Client: client, Opts: opts}
}

func (c *DefaultGitHubClient) GetOrganisationRunnerGroups(ctx context.Context, orgName string) ([]*github.RunnerGroup, error) {
	nextPage := 1
	var allGroups []*github.RunnerGroup

	for nextPage > 0 {
		runnerGroups, response, err := c.Client.Actions.ListOrganizationRunnerGroups(ctx, orgName, &github.ListOrgRunnerGroupOptions{
			ListOptions: github.ListOptions{
				Page:    nextPage,
				PerPage: pageSize,
			},
		})

		if err != nil {
			return nil, err
		}

		if response.StatusCode != 200 {
			return nil, errors.New("unexpected response from GitHub API: " + strconv.Itoa(response.StatusCode))
		}

		allGroups = append(allGroups, runnerGroups.RunnerGroups...)
		nextPage = response.NextPage
	}

	return allGroups, nil
}

func (c *DefaultGitHubClient) GetEnterpriseRunners(ctx context.Context, enterpriseName string) ([]*github.Runner, error) {
	var enterpriseRunners []*github.Runner
	var nextPage = 1

	for nextPage > 0 {
		runners, response, err := c.Client.Enterprise.ListRunners(ctx, enterpriseName, &github.ListOptions{
			Page:    nextPage,
			PerPage: pageSize,
		})

		if err != nil {
			return nil, err
		}

		if response.StatusCode != 200 {
			return nil, errors.New("unexpected response from GitHub API: " + strconv.Itoa(response.StatusCode))
		}

		enterpriseRunners = append(enterpriseRunners, runners.Runners...)
		nextPage = response.NextPage
	}

	return enterpriseRunners, nil
}

func (c *DefaultGitHubClient) GetGroupRunners(ctx context.Context, groupID int64, orgName string) ([]*github.Runner, error) {
	var groupRunners []*github.Runner
	var nextPage = 1

	for nextPage > 0 {
		runners, response, err := c.Client.Actions.ListRunnerGroupRunners(ctx, orgName, groupID, &github.ListOptions{
			Page:    nextPage,
			PerPage: pageSize,
		})

		if err != nil {
			return nil, err
		}

		if response.StatusCode != 200 {
			return nil, errors.New("unexpected response from GitHub API: " + strconv.Itoa(response.StatusCode))
		}

		groupRunners = append(groupRunners, runners.Runners...)
		nextPage = response.NextPage
	}

	return groupRunners, nil
}

func (c *DefaultGitHubClient) GetActionsBillingOrg(ctx context.Context, org string) (*github.ActionBilling, error) {
	billing, _, err := c.Client.Billing.GetActionsBillingOrg(ctx, org)
	return billing, err
}

func (c *DefaultGitHubClient) GetActionsBillingUser(ctx context.Context, user string) (*github.ActionBilling, error) {
	billing, _, err := c.Client.Billing.GetActionsBillingUser(ctx, user)
	return billing, err
}
