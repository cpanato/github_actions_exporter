package server_test

import (
	"context"
	"errors"
	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/log"
	"github.com/google/go-github/v47/github"
	"github.com/stretchr/testify/assert"
	"os"
	"sync"
	"testing"
	"time"
)

var (
	logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	opts   = server.Opts{
		GitHubOrg:             "gh-org",
		GitHubEnterprise:      "gh-enterprise",
		RunnersAPIPollSeconds: 1,
		RunnersMetricsEnabled: true,
		GitHubAPIToken:        "fake",
	}
)

const (
	offline = "offline"
	online  = "online"
)

type key struct {
	runnerGroup string
	busy        bool
	status      string
}

type TestGitHubClient struct {
	runnerGroups      []*github.RunnerGroup
	runnersPerGroup   map[int64][]*github.Runner
	enterpriseRunners []*github.Runner

	getRunnerGroupsError      error
	getEnterpriseRunnersError error
	getGroupRunnersError      error
}

func (t *TestGitHubClient) GetOrganisationRunnerGroups(ctx context.Context, orgName string) ([]*github.RunnerGroup, error) {
	return t.runnerGroups, t.getRunnerGroupsError
}

func (t *TestGitHubClient) GetEnterpriseRunners(ctx context.Context, enterpriseName string) ([]*github.Runner, error) {
	return t.enterpriseRunners, t.getEnterpriseRunnersError
}

func (t *TestGitHubClient) GetGroupRunners(ctx context.Context, groupID int64, orgName string) ([]*github.Runner, error) {
	return t.runnersPerGroup[groupID], t.getGroupRunnersError
}

func (t *TestGitHubClient) GetActionsBillingOrg(ctx context.Context, org string) (*github.ActionBilling, error) {
	return nil, errors.New("GetActionsBillingOrg should not be called")
}

func (t *TestGitHubClient) GetActionsBillingUser(ctx context.Context, user string) (*github.ActionBilling, error) {
	return nil, errors.New("GetActionsBillingUser should not be called")
}

type TestRunnerObserver struct {
	// Map of busy -> status -> runner group to hold precise counts per label combination
	metrics *sync.Map
}

func (t *TestRunnerObserver) ResetRegisteredRunnersTotal() {
	t.metrics.Range(func(key, value any) bool {
		t.metrics.Delete(key)
		return true
	})
}

func (t *TestRunnerObserver) IncreaseRegisteredRunnersTotal(busy bool, status string, runnerGroup string) {
	k := key{busy: busy, status: status, runnerGroup: runnerGroup}
	value, found := t.metrics.Load(k)
	if !found {
		t.metrics.Store(k, 1)
	} else {
		t.metrics.Store(k, value.(int)+1)
	}
}

func TestRunnersMetricsExporter_collectRunnersInformation(t *testing.T) {
	ghClient := TestGitHubClient{
		runnerGroups: []*github.RunnerGroup{runnerGroup(1, "group_one"), runnerGroup(2, "group_two")},
		runnersPerGroup: map[int64][]*github.Runner{
			// Busy but offline - those are returned from the API sometimes
			1: {runner(1, "i-1", true, offline)},
			2: {runner(1, "i-a", false, online), runner(2, "i-b", false, online), runner(3, "i-c", false, online)},
		},
		enterpriseRunners: []*github.Runner{runner(1, "e-1", false, offline), runner(2, "e-2", false, online)},
	}

	observer := TestRunnerObserver{metrics: &sync.Map{}}
	exporter := server.NewRunnersMetricsExporter(logger, opts, &ghClient, &observer)

	err := exporter.Start(context.Background())
	assert.NoError(t, err, "exporter could not be started")

	// Time for metrics to be retrieved by the ticket
	time.Sleep(time.Millisecond * 1200)

	value, found := observer.metrics.Load(key{runnerGroup: "group_one", busy: true, status: offline})
	assert.True(t, found)
	assert.Equal(t, 1, value)

	value, found = observer.metrics.Load(key{runnerGroup: "group_two", busy: false, status: online})
	assert.True(t, found)
	assert.Equal(t, 3, value)

	value, found = observer.metrics.Load(key{runnerGroup: "gh-enterprise", busy: false, status: offline})
	assert.True(t, found)
	assert.Equal(t, 1, value)

	value, found = observer.metrics.Load(key{runnerGroup: "gh-enterprise", busy: false, status: online})
	assert.True(t, found)
	assert.Equal(t, 1, value)
}

func TestRunnersMetricsExporter_StartError(t *testing.T) {

	tests := []struct {
		name           string
		incompleteOpts server.Opts
		expectedError  string
	}{
		{
			name: "Start_noOrgName", expectedError: "github org not configured", incompleteOpts: server.Opts{
				GitHubOrg:      "",
				GitHubAPIToken: "",
			}},
		{
			name: "Start_noAPIToken", expectedError: "github token not configured", incompleteOpts: server.Opts{
				GitHubOrg:      "gh-org",
				GitHubAPIToken: "",
			}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exporter := server.NewRunnersMetricsExporter(logger, test.incompleteOpts, nil, nil)

			err := exporter.Start(context.Background())
			assert.ErrorContains(t, err, test.expectedError)
		})
	}
}

// This test validates that existing metrics are cleared in the case of an API error to not produce misleading data
func TestRunnersMetricsExporter_collectRunnersInformationApiError(t *testing.T) {
	tests := []struct {
		name     string
		ghClient server.GitHubClient
	}{
		{
			name: "getRunnerGroups_error", ghClient: &TestGitHubClient{
				getRunnerGroupsError: errors.New("expected API error"),
			}},
		{
			name: "getGroupRunners_error", ghClient: &TestGitHubClient{
				getGroupRunnersError: errors.New("expected API error"),
			}},
		{
			name: "getEnterpriseRunners_error", ghClient: &TestGitHubClient{
				getEnterpriseRunnersError: errors.New("expected API error"),
			}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Start with existing metrics to ensure they are cleared when errors occur
			metrics := &sync.Map{}
			k := key{runnerGroup: "g1", busy: true, status: "offline"}
			metrics.Store(k, 3)
			observer := TestRunnerObserver{metrics: metrics}
			exporter := server.NewRunnersMetricsExporter(logger, opts, test.ghClient, &observer)

			err := exporter.Start(context.Background())
			assert.NoError(t, err, "exporter could not be started")

			// Time for metrics to be retrieved by the ticket
			time.Sleep(time.Millisecond * 1200)

			_, found := observer.metrics.Load(k)
			assert.False(t, found)
		})
	}
}

func runner(id int64, name string, busy bool, status string) *github.Runner {
	return &github.Runner{
		ID:     github.Int64(id),
		Name:   github.String(name),
		OS:     github.String("linux"),
		Status: github.String(status),
		Busy:   github.Bool(busy),
		Labels: []*github.RunnerLabels{},
	}
}

func runnerGroup(id int64, name string) *github.RunnerGroup {
	return &github.RunnerGroup{
		ID:                       github.Int64(id),
		Name:                     github.String(name),
		Visibility:               github.String("public"),
		Default:                  github.Bool(false),
		SelectedRepositoriesURL:  github.String("https://fake-url/repos"),
		RunnersURL:               github.String("https://fake-url/id/runners"),
		Inherited:                github.Bool(false),
		AllowsPublicRepositories: github.Bool(false),
	}
}
