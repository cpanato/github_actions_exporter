# github-exporter

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square) ![AppVersion: 0.8.0](https://img.shields.io/badge/AppVersion-0.8.0-informational?style=flat-square)

GitHub exporter

**Homepage:** <https://github.com/cpanato/github_actions_exporter>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| cpanato |  |  |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| config.org | string | `""` |  |
| config.pollSeconds | int | `30` |  |
| deployment.affinity | object | `{}` |  |
| deployment.image.digest | string | `"sha256:b0d50195686567de51b1ede25e2a6538719921f1a04a3cc763d89184ef86f22c"` |  |
| deployment.image.pullPolicy | string | `"IfNotPresent"` |  |
| deployment.image.repository | string | `"ghcr.io/cpanato/github_actions_exporter"` |  |
| deployment.image.tag | string | `"v0.8.0"` |  |
| deployment.nodeSelector | object | `{}` |  |
| deployment.podAnnotations | object | `{}` |  |
| deployment.podLabels | object | `{}` |  |
| deployment.replicaCount | int | `1` |  |
| deployment.resources | object | `{}` |  |
| deployment.tolerations | list | `[]` |  |
| fullnameOverride | string | `""` |  |
| ingress.annotations | object | `{}` |  |
| ingress.enabled | bool | `false` |  |
| ingress.extraLabels | object | `{}` |  |
| ingress.host | string | `nil` |  |
| labels | object | `{}` |  |
| nameOverride | string | `""` |  |
| prometheus.gcpManagedPrometheus.enabled | bool | `false` |  |
| prometheus.serviceMonitor.additionalLabels.app | string | `"github-exporter"` |  |
| prometheus.serviceMonitor.enabled | bool | `false` |  |
| prometheus.serviceMonitor.interval | string | `"5m"` |  |
| prometheus.serviceMonitor.metricRelabelings | object | `{}` |  |
| prometheus.serviceMonitor.namespace | string | `"monitoring"` |  |
| prometheus.serviceMonitor.scrapeTimeout | string | `"2m"` |  |
| secret.existingSecretName | string | `""` |  |
| secret.githubToken | string | `""` |  |
| secret.githubWebhookToken | string | `""` |  |
| service.annotationsIngress | object | `{}` |  |
| service.annotationsMetrics | object | `{}` |  |
| service.extraLabels | object | `{}` |  |
| service.portHttp | int | `8065` |  |
| service.portMetrics | int | `9101` |  |
| service.type | string | `"ClusterIP"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
