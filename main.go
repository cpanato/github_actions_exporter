package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cpanato/github_actions_exporter/internal/server"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddress  = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9101").String()
	metricsPath    = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	ghWebHookPath  = kingpin.Flag("web.gh-webhook-path", "Path that will be called by the GitHub webhook.").Default("/gh_event").String()
	gitHubToken    = kingpin.Flag("gh.github-webhook-token", "GitHub Webhook Token.").Default("").String()
	gitHubAPIToken = kingpin.Flag("gh.github-api-token", "GitHub API Token.").Default("").String()
	gitHubOrg      = kingpin.Flag("gh.github-org", "GitHub Organization.").Default("").String()
	gitHubUser     = kingpin.Flag("gh.github-user", "GitHub User.").Default("").String()
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

	level.Info(logger).Log("msg", "Starting ghactions_exporter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	if err := validateFlags(*gitHubAPIToken, *gitHubToken, *gitHubOrg, *gitHubUser); err != nil {
		level.Error(logger).Log("msg", "Missing configure flags", "err", err)
		os.Exit(1)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	srv := server.NewServer(logger, server.ServerOpts{
		WebhookPath:    *ghWebHookPath,
		ListenAddress:  *listenAddress,
		MetricsPath:    *metricsPath,
		GitHubToken:    *gitHubToken,
		GitHubAPIToken: *gitHubAPIToken,
		GitHubUser:     *gitHubUser,
		GitHubOrg:      *gitHubOrg,
	})
	go func() {
		err := srv.Serve(context.Background())
		if err != nil {
			level.Error(logger).Log("msg", "Server closed", "err", err)
		} else {
			level.Info(logger).Log("msg", "Server closed")
		}
	}()

	level.Info(logger).Log("msg", fmt.Sprintf("Signal received: %v. Exiting...", <-signalChan))
	err := srv.Shutdown(context.Background())
	if err != nil {
		level.Error(logger).Log("msg", "Error occurred while closing the server", "err", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func validateFlags(apiToken, token, org, user string) error {
	if token == "" {
		return errors.New("Please configure the GitHub Webhook Token")
	}

	if apiToken == "" {
		return errors.New("Please configure the GitHub API Token")
	}

	if org == "" && user == "" {
		fmt.Print(org, user)
		return errors.New("Please configure the GitHub Organization or GitHub User ")
	}

	return nil
}
