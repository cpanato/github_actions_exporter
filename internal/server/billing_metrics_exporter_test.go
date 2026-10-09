package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_summarizeActionsUsage(t *testing.T) {
	items := []*github.UsageItem{
		// 100 minutes, 60 of them covered by the included quota (net 40 * 0.008).
		{Product: "Actions", SKU: "Actions Linux", UnitType: "minutes", Quantity: 100, PricePerUnit: 0.008, GrossAmount: 0.8, DiscountAmount: 0.48, NetAmount: 0.32},
		{Product: "Actions", SKU: "Actions Linux", UnitType: "minutes", Quantity: 20, PricePerUnit: 0.008, GrossAmount: 0.16, DiscountAmount: 0.16, NetAmount: 0},
		{Product: "actions", SKU: "actions_macos", UnitType: "minutes", Quantity: 10, PricePerUnit: 0.08, GrossAmount: 0.8, NetAmount: 0.8},
		{Product: "Actions", SKU: "Actions Linux 4-core", UnitType: "minutes", Quantity: 5, PricePerUnit: 0.016, GrossAmount: 0.08, NetAmount: 0.08},
		// Not Actions minutes: ignored.
		{Product: "Actions", SKU: "Actions storage", UnitType: "GigabyteHours", Quantity: 500, PricePerUnit: 0.0003, NetAmount: 0.15},
		{Product: "Packages", SKU: "Packages data transfer", UnitType: "minutes", Quantity: 7},
		nil,
	}

	got := summarizeActionsUsage(items)

	assert.InDelta(t, 135, got.totalMinutes, 1e-9)
	assert.InDelta(t, 40+10+5, got.paidMinutes, 1e-9)
	assert.Equal(t, map[string]float64{
		"UBUNTU":               120,
		"MACOS":                10,
		"actions_linux_4-core": 5,
	}, got.minutesByHostType)
}

func Test_summarizeActionsUsage_NoUsage(t *testing.T) {
	got := summarizeActionsUsage(nil)

	assert.Zero(t, got.totalMinutes)
	assert.Zero(t, got.paidMinutes)
	assert.Empty(t, got.minutesByHostType)
}

func Test_currentMonthUsageOptions(t *testing.T) {
	// 23:30 UTC on Jan 31st is already February in UTC+2, the report must use UTC.
	now := time.Date(2026, time.January, 31, 23, 30, 0, 0, time.UTC).In(time.FixedZone("UTC+2", 2*60*60))

	got := currentMonthUsageOptions(now)

	assert.Equal(t, 2026, *got.Year)
	assert.Equal(t, 1, *got.Month)
}

func newTestBillingExporter(t *testing.T, handler http.HandlerFunc) *BillingMetricsExporter {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	baseURL := ts.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
	require.NoError(t, err)

	return &BillingMetricsExporter{
		GHClient: client,
		Logger:   slog.New(slog.DiscardHandler),
		Opts:     Opts{GitHubOrg: "test-org", GitHubUser: "test-user"},
	}
}

func Test_BillingMetricsExporter_collectOrgBilling(t *testing.T) {
	exporter := newTestBillingExporter(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/organizations/test-org/settings/billing/usage", r.URL.Path)
		assert.NotEmpty(t, r.URL.Query().Get("year"))
		assert.NotEmpty(t, r.URL.Query().Get("month"))
		_, _ = w.Write([]byte(`{"usageItems":[
			{"date":"2026-10-01","product":"Actions","sku":"Actions Linux","quantity":100,"unitType":"minutes","pricePerUnit":0.008,"grossAmount":0.8,"discountAmount":0.4,"netAmount":0.4},
			{"date":"2026-10-02","product":"Actions","sku":"Actions Windows","quantity":50,"unitType":"minutes","pricePerUnit":0.016,"grossAmount":0.8,"discountAmount":0.8,"netAmount":0}
		]}`))
	})

	exporter.collectOrgBilling(context.Background())

	assert.InDelta(t, 150, testutil.ToFloat64(totalMinutesUsedActions.WithLabelValues("test-org", "")), 1e-9)
	assert.InDelta(t, 50, testutil.ToFloat64(totalPaidMinutesActions.WithLabelValues("test-org", "")), 1e-9)
	assert.InDelta(t, 100, testutil.ToFloat64(includedMinutesUsedActions.WithLabelValues("test-org", "")), 1e-9)
	assert.InDelta(t, 100, testutil.ToFloat64(totalMinutesUsedByHostTypeActions.WithLabelValues("test-org", "", "UBUNTU")), 1e-9)
	assert.InDelta(t, 50, testutil.ToFloat64(totalMinutesUsedByHostTypeActions.WithLabelValues("test-org", "", "WINDOWS")), 1e-9)
}

func Test_BillingMetricsExporter_collectUserBilling(t *testing.T) {
	exporter := newTestBillingExporter(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/users/test-user/settings/billing/usage", r.URL.Path)
		_, _ = w.Write([]byte(`{"usageItems":[
			{"date":"2026-10-01","product":"Actions","sku":"Actions Linux","quantity":30,"unitType":"minutes","pricePerUnit":0.008,"grossAmount":0.24,"discountAmount":0.24,"netAmount":0}
		]}`))
	})

	exporter.collectUserBilling(context.Background())

	assert.InDelta(t, 30, testutil.ToFloat64(totalMinutesUsedActions.WithLabelValues("", "test-user")), 1e-9)
	assert.InDelta(t, 0, testutil.ToFloat64(totalPaidMinutesActions.WithLabelValues("", "test-user")), 1e-9)
	assert.InDelta(t, 30, testutil.ToFloat64(totalMinutesUsedByHostTypeActions.WithLabelValues("", "test-user", "UBUNTU")), 1e-9)
}

func Test_BillingMetricsExporter_collectOrgBilling_DropsStaleHostTypes(t *testing.T) {
	const org = "stale-org"
	totalMinutesUsedByHostTypeActions.Reset()
	responses := []string{
		`{"usageItems":[{"product":"Actions","sku":"Actions macOS","quantity":10,"unitType":"minutes","pricePerUnit":0.08,"netAmount":0.8}]}`,
		`{"usageItems":[{"product":"Actions","sku":"Actions Linux","quantity":5,"unitType":"minutes","pricePerUnit":0.008,"netAmount":0.04}]}`,
	}
	call := 0
	exporter := newTestBillingExporter(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(responses[call]))
		call++
	})
	exporter.Opts.GitHubOrg = org

	exporter.collectOrgBilling(context.Background())
	assert.Equal(t, 1, testutil.CollectAndCount(totalMinutesUsedByHostTypeActions))

	exporter.collectOrgBilling(context.Background())
	assert.Equal(t, 1, testutil.CollectAndCount(totalMinutesUsedByHostTypeActions))
	assert.InDelta(t, 5, testutil.ToFloat64(totalMinutesUsedByHostTypeActions.WithLabelValues(org, "", "UBUNTU")), 1e-9)
}

func Test_BillingMetricsExporter_StartOrgBilling_StopsOnContextCancel(t *testing.T) {
	var logs syncBuffer
	exporter := newTestBillingExporter(t, func(http.ResponseWriter, *http.Request) {})
	exporter.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	exporter.Opts = Opts{GitHubOrg: "test-org", GitHubAPIToken: "token", BillingAPIPollSeconds: 3600}

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, exporter.StartOrgBilling(ctx))
	cancel()

	assert.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "stopped polling for org billing metrics")
	}, 5*time.Second, 10*time.Millisecond)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func Test_BillingMetricsExporter_collectOrgBilling_APIErrorKeepsMetrics(t *testing.T) {
	exporter := newTestBillingExporter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	exporter.Opts.GitHubOrg = "error-org"
	totalMinutesUsedActions.WithLabelValues("error-org", "").Set(42)

	exporter.collectOrgBilling(context.Background())

	assert.InDelta(t, 42, testutil.ToFloat64(totalMinutesUsedActions.WithLabelValues("error-org", "")), 1e-9)
}
