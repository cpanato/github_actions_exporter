package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	GitHubAPIToken string
	GitHubOrg      string
	GitHubUser     string
}

type Server struct {
	logger   log.Logger
	server   *http.Server
	exporter *GHActionExporter
	opts     Opts
}

func NewServer(logger log.Logger, opts Opts) *Server {
	mux := http.NewServeMux()

	httpServer := &http.Server{
		Handler: mux,
	}

	exporter := NewGHActionExporter(logger, opts)
	server := &Server{
		logger:   logger,
		server:   httpServer,
		exporter: exporter,
		opts:     opts,
	}

	mux.Handle(opts.MetricsPath, promhttp.Handler())
	mux.HandleFunc(opts.WebhookPath, server.exporter.HandleGHWebHook)
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
