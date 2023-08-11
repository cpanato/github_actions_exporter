package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/go-github/v47/github"
	"golang.org/x/oauth2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
)

type Opts struct {
	MetricsPath   string
	ListenAddress string
	WebhookPath   string
	// GitHub webhook token.
	GitHubToken string
	// GitHub API token.
	GitHubAPIToken        string
	GitHubOrg             string
	GitHubEnterprise      string
	GitHubUser            string
	BillingAPIPollSeconds int
	RunnersAPIPollSeconds int
	BillingMetricsEnabled bool
	RunnersMetricsEnabled bool
}

type Server struct {
	logger log.Logger
	server *http.Server
	opts   Opts
}

func NewServer(logger log.Logger, opts Opts) *Server {
	mux := http.NewServeMux()

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: opts.GitHubAPIToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	ghClient := NewGitHubClient(&opts, github.NewClient(tc))
	observer := PrometheusObserver{}

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if opts.BillingMetricsEnabled {
		billingExporter := NewBillingMetricsExporter(logger, opts, ghClient)
		err := billingExporter.StartOrgBilling(context.TODO())
		if err != nil {
			_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting org billing: %v", err))
		}
		err = billingExporter.StartUserBilling(context.TODO())
		if err != nil {
			_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting user billing: %v", err))
		}
	} else {
		_ = level.Info(logger).Log("msg", "billing metrics are disabled")
	}

	if opts.RunnersMetricsEnabled {
		runnersExporter := NewRunnersMetricsExporter(logger, opts, ghClient, &observer)
		err := runnersExporter.Start(context.TODO())
		if err != nil {
			_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting runners: %v", err))
		}
	} else {
		_ = level.Info(logger).Log("msg", "runners metrics are disabled")
	}

	workflowExporter := NewWorkflowMetricsExporter(logger, opts, &observer)
	server := &Server{
		logger: logger,
		server: httpServer,
		opts:   opts,
	}

	mux.Handle(opts.MetricsPath, promhttp.Handler())
	mux.HandleFunc(opts.WebhookPath, workflowExporter.HandleGHWebHook)
	mux.HandleFunc("/", server.handleRoot)

	return server
}

func (s *Server) Serve(ctx context.Context) error {
	listener, err := getListener(s.opts.ListenAddress, s.logger)
	if err != nil {
		return fmt.Errorf("get listener: %w", err)
	}

	_ = level.Info(s.logger).Log("msg", "GitHub Actions Prometheus Exporter has successfully started")
	err = s.server.Serve(listener)

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server closed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(`<html>
		<head><title>GitHub Actions Exporter</title></head>
		<body>
		<h1>GitHub Actions Exporter</h1>
		<p> ` + version.Print("ghactions_exporter") + `  </p>
		<p><a href='` + s.opts.MetricsPath + `'>Metrics</a></p>
		</body>
		</html>
	`))
}

func getListener(listenAddress string, logger log.Logger) (net.Listener, error) {
	var listener net.Listener
	var err error

	if strings.HasPrefix(listenAddress, "unix:") {
		path, _, pathError := parseUnixSocketAddress(listenAddress)
		if pathError != nil {
			return listener, fmt.Errorf("parsing unix domain socket listen address %s failed: %w", listenAddress, pathError)
		}
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	} else {
		listener, err = net.Listen("tcp", listenAddress)
	}

	if err != nil {
		return listener, err
	}

	_ = level.Info(logger).Log("msg", fmt.Sprintf("Listening on %s", listenAddress))
	return listener, nil
}

func parseUnixSocketAddress(address string) (string, string, error) {
	addressParts := strings.Split(address, ":")
	addressPartsLength := len(addressParts)

	if addressPartsLength > 3 || addressPartsLength < 1 {
		return "", "", fmt.Errorf("address for unix domain socket has wrong format")
	}

	unixSocketPath := addressParts[1]
	requestPath := ""
	if addressPartsLength == 3 {
		requestPath = addressParts[2]
	}

	return unixSocketPath, requestPath, nil
}
