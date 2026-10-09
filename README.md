# GitHub Actions Exporter

Prometheus exporter exposing [GitHub Actions](https://github.com/features/actions) metrics:

- workflow job and workflow run metrics (durations, queue time, status counts), built from GitHub webhook events
- GitHub Actions billing metrics (minutes used, paid and per runner type), polled from the GitHub API

## Getting Started

This exporter receives webhook events from GitHub.

If you want to collect metrics from a GitHub repository or organization you will need to create a webhook
in GitHub.

You need to select the `Workflow jobs` and `Workflow runs` events and set your secret (the same one you start your exporter with, see below).

The exporter listens on two addresses: the webhook is received on `--web.listen-address-ingress` (`:8065` by default) and the metrics are exposed on `--web.listen-address` (`:9101` by default).
The webhook will call the `/gh_event` path on your ingress endpoint by default. You can change this with the `--web.gh-webhook-path` option.

![gh_webook](./assets/gh_webhook.png)

Also it collects the Action Billing metrics, for that you will need to setup a GitHub API Access Token

The metrics are built from the GitHub [billing usage report](https://docs.github.com/rest/billing/usage) of the current month (enhanced billing platform), because the legacy Actions billing API was retired by GitHub:

- `actions_total_minutes_used_minutes`: all Actions minutes used.
- `actions_total_paid_minutes`: minutes that were charged.
- `actions_included_minutes`: minutes covered by the included quota or discounts (total minus paid). It is no longer the plan allowance, which the new API does not report.
- `actions_total_minutes_used_by_host_minutes`: minutes per runner type. The `host_type` label is `UBUNTU`, `WINDOWS` or `MACOS` for the standard runners, and the SKU (e.g. `actions_linux_4-core`) for larger runners.

When configuring for an organization, the Access token must belong to an administrator of the organization and the organization must be on the enhanced billing platform. When configuring for an user, the token must belong to that user.

### Metrics

Webhook metrics:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `workflow_job_duration_seconds` | histogram | `org`, `repo`, `branch`, `state`, `runner_group`, `workflow_name`, `job_name` | Time that a workflow job took to reach a given state (`queued`, `in_progress`). |
| `workflow_job_duration_seconds_total` | counter | `org`, `repo`, `branch`, `status`, `conclusion`, `runner_group`, `workflow_name`, `job_name` | The total duration of jobs. |
| `workflow_job_status_count` | counter | `org`, `repo`, `branch`, `status`, `conclusion`, `runner_group`, `workflow_name`, `job_name` | Count of workflow job events. |
| `workflow_execution_time_seconds` | histogram | `org`, `repo`, `branch`, `workflow_name`, `conclusion` | Time that a workflow took to run. |
| `workflow_status_count` | counter | `org`, `repo`, `branch`, `status`, `conclusion`, `workflow_name` | Count of the occurrences of different workflow states. |

### Example queries

Time a job waits for a runner (queued until started) per runner group, useful to compare how fast runner groups pick up jobs:

```promql
sum by (runner_group) (rate(workflow_job_duration_seconds_sum{state="queued"}[5m]))
/
sum by (runner_group) (rate(workflow_job_duration_seconds_count{state="queued"}[5m]))
```

95th percentile of the same:

```promql
histogram_quantile(0.95, sum by (le, runner_group) (rate(workflow_job_duration_seconds_bucket{state="queued"}[5m])))
```

### Flags

| Flag | Environment variable | Default | Description |
|------|----------------------|---------|-------------|
| `--web.listen-address` | | `:9101` | Address to listen on for metrics. |
| `--web.listen-address-ingress` | | `:8065` | Address to listen on for the webhook and the web interface. |
| `--web.telemetry-path` | | `/metrics` | Path under which to expose metrics. |
| `--web.gh-webhook-path` | | `/gh_event` | Path that will be called by the GitHub webhook. |
| `--gh.github-webhook-token` | `GITHUB_WEBHOOK_TOKEN` | | GitHub webhook secret (required). |
| `--gh.github-api-token` | `GITHUB_API_TOKEN` | | GitHub API token, only needed for the billing metrics. |
| `--gh.github-org` | `GITHUB_ORG` | | GitHub organization to collect the billing metrics for. |
| `--gh.github-user` | | | GitHub user to collect the billing metrics for. |
| `--gh.billing-poll-seconds` | `BILLING_POLL_SECONDS` | `5` | Frequency at which to poll the billing API. |
| `--log.level` | | `info` | Only log messages with the given severity or above (`debug`, `info`, `warn`, `error`). |
| `--log.format` | | `logfmt` | Output format of log messages (`logfmt`, `json`). |

Run `github_actions_exporter --help` for the complete list.

### Prerequisites

To run this project, you will need a [working Go environment](https://golang.org/doc/install).

### Installing

```bash
go install github.com/cpanato/github_actions_exporter@latest
```

## Building

Build the sources with

```bash
make build
```

## Run the binary

```bash
./github_actions_exporter --gh.github-webhook-token="MY_TOKEN" --gh.github-api-token="Accesstoken" --gh.github-org="honk_org"
```

## Docker

You can deploy this exporter using the [ghcr.io/cpanato/github_actions_exporter](https://github.com/users/cpanato/packages/container/package/github_actions_exporter) Docker image.

For example (replace `<version>` with a [release](https://github.com/cpanato/github_actions_exporter/releases) tag, e.g. `v0.10.1`):

```bash
docker pull ghcr.io/cpanato/github_actions_exporter:<version>
docker run -d -p 9101:9101 -p 8065:8065 ghcr.io/cpanato/github_actions_exporter:<version> --gh.github-webhook-token="1234567890token" --gh.github-api-token="Accesstoken" --gh.github-org="honk_org"
```

Port `9101` serves the metrics and port `8065` receives the GitHub webhooks.

## Helm chart

A Helm chart is available in [charts/github-exporter](./charts/github-exporter).

## Testing

### Running unit tests

```bash
make test
```

### Manual testing

```bash
cd example/
export GITHUB_WEBHOOK_TOKEN="..."
export GITHUB_API_TOKEN="..."
export GITHUB_ORG="..."
docker-compose up --build
```

Open Prometheus at http://localhost:9090 and explore the available metrics.

## Contributing

Refer to [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

Apache License 2.0, see [LICENSE](./LICENSE).
