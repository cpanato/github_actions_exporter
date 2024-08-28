package server

import (
	"context"
	"errors"
	"fmt"
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
	MetricsPath          string
	ListenAddressMetrics string
	ListenAddressIngress string
	WebhookPath          string
	// GitHub webhook token.
	GitHubToken string
	// GitHub API token.
	GitHubAPIToken          string
	GitHubOrg               string
	GitHubUser              string
	GitHubRepo              string
	BillingAPIPollSeconds   int
	WorkflowsAPIPollSeconds int
}

type Server struct {
	logger                  log.Logger
	serverMetrics           *http.Server
	serverIngress           *http.Server
	workflowMetricsExporter *WorkflowMetricsExporter
	billingExporter         *BillingMetricsExporter
	opts                    Opts
}

func NewServer(logger log.Logger, opts Opts) *Server {
	muxMetrics := http.NewServeMux()
	httpServerMetrics := &http.Server{
		Handler:           muxMetrics,
		ReadHeaderTimeout: 10 * time.Second,
	}

	billingExporter := NewBillingMetricsExporter(logger, opts)
	err := billingExporter.StartOrgBilling(context.TODO())
	if err != nil {
		_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting org billing: %v", err))
	}
	err = billingExporter.StartUserBilling(context.TODO())
	if err != nil {
		_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting user billing: %v", err))
	}

	workflowMetricsExporter := NewApiWorkflowMetricsExporter(logger, opts)
	err = workflowMetricsExporter.StartWorkflowApiPolling(context.TODO())
	if err != nil {
		_ = level.Info(logger).Log("msg", fmt.Sprintf("not exporting workflow metrics %v", err))
	}

	muxIngress := http.NewServeMux()
	httpServerIngress := &http.Server{
		Handler:           muxIngress,
		ReadHeaderTimeout: 10 * time.Second,
	}

	workflowExporter := NewWorkflowMetricsExporter(logger, opts)
	server := &Server{
		logger:                  logger,
		serverMetrics:           httpServerMetrics,
		serverIngress:           httpServerIngress,
		workflowMetricsExporter: workflowExporter,
		billingExporter:         billingExporter,
		opts:                    opts,
	}

	muxMetrics.Handle(opts.MetricsPath, promhttp.Handler())

	muxIngress.HandleFunc("/", server.handleRoot)
	muxIngress.HandleFunc(opts.WebhookPath, workflowExporter.HandleGHWebHook)

	return server
}

func (s *Server) Serve(ctx context.Context) error {
	listenerMetrics, err := getListener(s.opts.ListenAddressMetrics, s.logger)
	if err != nil {
		return fmt.Errorf("get listener: %w", err)
	}

	listenerIgress, err := getListener(s.opts.ListenAddressIngress, s.logger)
	if err != nil {
		return fmt.Errorf("get listener: %w", err)
	}

	_ = level.Info(s.logger).Log("msg", "GitHub Actions Prometheus Exporter Metrics has successfully started")
	go func() {
		_ = s.serverMetrics.Serve(listenerMetrics)
	}()

	_ = level.Info(s.logger).Log("msg", "GitHub Actions Prometheus Exporter Ingress has successfully started")
	err = s.serverIngress.Serve(listenerIgress)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server ingress closed: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.serverMetrics.Shutdown(ctx)
	if err != nil {
		return err
	}

	err = s.serverIngress.Shutdown(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(`<html>
		<head><title>GitHub Actions Exporter</title></head>
		<body>
		<h1>GitHub Actions Exporter</h1>
		<p> ` + version.Print("ghactions_exporter") + `  </p>
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
