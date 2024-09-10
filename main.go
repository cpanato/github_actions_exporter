package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/fernride/github_actions_exporter/internal/server"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

var (
	listenAddressMetrics          = kingpin.Flag("web.listen-address", "Address to listen on for metrics.").Default(":9101").String()
	listenAddressIngress          = kingpin.Flag("web.listen-address-ingress", "Address to listen on for web interface and receive webhook.").Default(":8065").String()
	metricsPath                   = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	ghWebHookPath                 = kingpin.Flag("web.gh-webhook-path", "Path that will be called by the GitHub webhook.").Default("/gh_event").String()
	githubWebhookToken            = kingpin.Flag("gh.github-webhook-token", "GitHub Webhook Token.").Envar("GITHUB_WEBHOOK_TOKEN").Default("").String()
	gitHubAPIToken                = kingpin.Flag("gh.github-api-token", "GitHub API Token.").Envar("GITHUB_API_TOKEN").Default("").String()
	gitHubOrg                     = kingpin.Flag("gh.github-org", "GitHub Organization.").Envar("GITHUB_ORG").Default("").String()
	gitHubUser                    = kingpin.Flag("gh.github-user", "GitHub User.").Default("").String()
	gitHubBillingPollingSeconds   = kingpin.Flag("gh.billing-poll-seconds", "Frequency at which to poll billing API.").Envar("BILLING_POLL_SECONDS").Default("120").Int()
	gitHubRepo                    = kingpin.Flag("gh.github-repo", "GitHub Repo.").Envar("GITHUB_REPO").String()
	gitHubWorkflowsPollingSeconds = kingpin.Flag("gh.workflows-poll-seconds", "Frequency at which to poll billing workflows.").Envar("WORKFLOWS_POLL_SECONDS").Default("60").Int()
)

func init() {
	prometheus.MustRegister(version.NewCollector("ghactions_exporter"))
}

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("ghactions_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	_ = level.Info(logger).Log("msg", "Starting ghactions_exporter", "version", version.Info())
	_ = level.Info(logger).Log("build_context", version.BuildContext())

	if err := validateFlags(*githubWebhookToken); err != nil {
		_ = level.Error(logger).Log("msg", "Missing configure flags", "err", err)
		os.Exit(1)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	srv := server.NewServer(logger, server.Opts{
		WebhookPath:             *ghWebHookPath,
		ListenAddressMetrics:    *listenAddressMetrics,
		ListenAddressIngress:    *listenAddressIngress,
		MetricsPath:             *metricsPath,
		GitHubToken:             *githubWebhookToken,
		GitHubAPIToken:          *gitHubAPIToken,
		GitHubUser:              *gitHubUser,
		GitHubOrg:               *gitHubOrg,
		GitHubRepo:              *gitHubRepo,
		BillingAPIPollSeconds:   *gitHubBillingPollingSeconds,
		WorkflowsAPIPollSeconds: *gitHubWorkflowsPollingSeconds,
	})
	go func() {
		err := srv.Serve(context.Background())
		if err != nil {
			_ = level.Error(logger).Log("msg", "Server closed", "err", err)
		} else {
			_ = level.Info(logger).Log("msg", "Server closed")
		}
	}()

	_ = level.Info(logger).Log("msg", fmt.Sprintf("Signal received: %v. Exiting...", <-signalChan))
	err := srv.Shutdown(context.Background())
	if err != nil {
		_ = level.Error(logger).Log("msg", "Error occurred while closing the server", "err", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func validateFlags(token string) error {
	if token == "" {
		return errors.New("please configure the GitHub Webhook Token")
	}
	return nil
}
