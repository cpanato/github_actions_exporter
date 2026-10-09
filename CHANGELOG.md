## Unreleased

**Breaking / action required**

- Billing metrics now use the GitHub billing usage report of the current month (#296). GitHub retired the legacy Actions billing API.
  - Metric names and labels are unchanged, but `actions_included_minutes` is now the number of minutes covered by the included quota or discounts (total minus paid). It is no longer the plan allowance, which the new API does not report.
  - `host_type` is still `UBUNTU`, `WINDOWS` or `MACOS` for the standard runners. Larger runners use their SKU name, e.g. `actions_linux_4-core`.
  - Organization billing requires a token of an organization administrator and an organization on the enhanced billing platform.
  - The report can be large for big organizations, consider increasing `BILLING_POLL_SECONDS` (default `5`).
- Logging moved from go-kit/log to `log/slog` (#295). `--log.level` and `--log.format` are unchanged, but log lines are now structured with `time`, `level`, `source` and `msg` keys, so anything parsing the old output may need an update.
- Release binaries and the checksums file are now signed with Cosign v3 as a single Sigstore bundle, `<artifact>.sigstore.json`, instead of separate `.sig` and `.pem` files (#294). Verify them with `cosign verify-blob --bundle <artifact>.sigstore.json ...`.

**Other changes**

- Build with Go 1.27.2 and update dependencies (`prometheus/client_golang` 1.25, `prometheus/common` 0.72, `go-github` v92).

## 0.3.0 / 2022-04-20

- Add more metrics
- Add tests
- Sign the binaries and images with cosign

## 0.2.0 / 2020-11-23

- Add support to show metrics regarding the Actions billing.

## 0.1.0 / 2020-11-23

- Initial release.
