package metrics

import (
	"context"

	"github.com/go-kit/log"
	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"

	"github.com/cpanato/github_actions_exporter/pkg/config"
)

type ExporterClient struct {
	GHClient           *github.Client
	Logger             log.Logger
	Opts               config.Opts
	PrometheusObserver WorkflowObserver
}

func NewClient(logger log.Logger, opts config.Opts) *ExporterClient {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.GitHubAPIToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &ExporterClient{
		Logger:             logger,
		Opts:               opts,
		GHClient:           client,
		PrometheusObserver: &PrometheusObserver{},
	}
}
