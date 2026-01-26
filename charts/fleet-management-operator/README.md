# Fleet Management Operator Helm Chart

Helm chart for deploying the Grafana Fleet Management Pipeline Operator to Kubernetes.

## Prerequisites

- Kubernetes 1.23+
- Helm 3.8+
- Grafana Cloud account with Fleet Management enabled
- Fleet Management API credentials (Stack ID and Cloud API token)

## Installing the Chart

### Quick Start

```bash
# Add the Helm repository
helm repo add fm-operator https://YOUR_USERNAME.github.io/fm-crd
helm repo update

# Install with minimum configuration
helm install fleet-management-operator fm-operator/fleet-management-operator \
  --namespace fleet-management-system \
  --create-namespace \
  --set fleetManagement.baseUrl='https://fleet-management-prod-us-central-0.grafana.net/pipeline.v1.PipelineService/' \
  --set fleetManagement.username='YOUR_STACK_ID' \
  --set fleetManagement.password='YOUR_GRAFANA_CLOUD_TOKEN'
```

### Install from Source

```bash
cd charts/fleet-management-operator

helm install fleet-management-operator . \
  --namespace fleet-management-system \
  --create-namespace \
  --set fleetManagement.baseUrl='https://fleet-management-prod-us-central-0.grafana.net/pipeline.v1.PipelineService/' \
  --set fleetManagement.username='12345' \
  --set fleetManagement.password='glc_xxxxx'
```

### Using a Values File

Create a `values-prod.yaml` file:

```yaml
image:
  repository: ghcr.io/grafana/fleet-management-operator
  tag: v0.1.0

fleetManagement:
  baseUrl: https://fleet-management-prod-us-central-0.grafana.net/pipeline.v1.PipelineService/
  username: "12345"
  password: "glc_xxxxx"

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

metrics:
  enabled: true
  service:
    serviceMonitor:
      enabled: true
```

Install with the values file:

```bash
helm install fleet-management-operator . \
  --namespace fleet-management-system \
  --create-namespace \
  -f values-prod.yaml
```

## Using Existing Secret

If you already have a secret with Fleet Management credentials:

```bash
kubectl create secret generic my-fleet-credentials \
  -n fleet-management-system \
  --from-literal=base-url='https://fleet-management-prod-us-central-0.grafana.net/pipeline.v1.PipelineService/' \
  --from-literal=username='12345' \
  --from-literal=password='glc_xxxxx'

helm install fleet-management-operator . \
  --namespace fleet-management-system \
  --set fleetManagement.existingSecret=my-fleet-credentials
```

## Configuration

The following table lists the configurable parameters and their default values.

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `fleet-management-operator` |
| `image.tag` | Container image tag | `dev-v1.0.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |

### Fleet Management API

| Parameter | Description | Default |
|-----------|-------------|---------|
| `fleetManagement.baseUrl` | Fleet Management API base URL | `""` (required) |
| `fleetManagement.username` | Grafana Cloud Stack ID | `""` (required) |
| `fleetManagement.password` | Grafana Cloud API token | `""` (required) |
| `fleetManagement.existingSecret` | Use existing secret for credentials | `""` |

### Deployment

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Metrics

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.port` | Metrics port | `8080` |
| `metrics.service.type` | Metrics service type | `ClusterIP` |
| `metrics.service.serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `metrics.service.serviceMonitor.interval` | Scrape interval | `30s` |

### Leader Election

| Parameter | Description | Default |
|-----------|-------------|---------|
| `leaderElection.enabled` | Enable leader election | `true` |

### RBAC

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Create RBAC resources | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` (auto-generated) |

## Upgrading the Chart

```bash
helm upgrade fleet-management-operator . \
  --namespace fleet-management-system \
  -f values-prod.yaml
```

## Uninstalling the Chart

```bash
helm uninstall fleet-management-operator --namespace fleet-management-system
```

**Note**: This will NOT delete the CRDs. To delete CRDs:

```bash
kubectl delete crd pipelines.fleetmanagement.grafana.com
```

## Examples

### Create a Pipeline

After installing the operator, create a Pipeline resource:

```yaml
apiVersion: fleetmanagement.grafana.com/v1alpha1
kind: Pipeline
metadata:
  name: prometheus-monitoring
  namespace: fleet-management-system
spec:
  contents: |
    prometheus.exporter.self "alloy" { }

    prometheus.scrape "alloy" {
      targets = prometheus.exporter.self.alloy.targets
      forward_to = [prometheus.remote_write.grafanacloud.receiver]
    }
  matchers:
    - collector.os=linux
    - environment=production
  enabled: true
  configType: Alloy
  source:
    type: Kubernetes
    namespace: production-cluster
```

### Enable Prometheus Monitoring

```yaml
metrics:
  enabled: true
  service:
    serviceMonitor:
      enabled: true
      additionalLabels:
        prometheus: kube-prometheus
```

### High Availability Setup

```yaml
replicaCount: 2
leaderElection:
  enabled: true

podDisruptionBudget:
  enabled: true
  minAvailable: 1

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: fleet-management-operator
        topologyKey: kubernetes.io/hostname
```

## Troubleshooting

### Check Operator Status

```bash
kubectl get pods -n fleet-management-system
kubectl logs -n fleet-management-system deployment/fleet-management-operator-controller-manager
```

### Verify CRD Installation

```bash
kubectl get crds pipelines.fleetmanagement.grafana.com
kubectl explain pipeline.spec
```

### Check Pipeline Status

```bash
kubectl get pipelines -A
kubectl describe pipeline <pipeline-name> -n <namespace>
```

## Support

For issues and questions:
- GitHub Issues: https://github.com/YOUR_USERNAME/fm-crd/issues
- Grafana Fleet Management Documentation: https://grafana.com/docs/grafana-cloud/monitor-infrastructure/fleet-management/
- Grafana Alloy Documentation: https://grafana.com/docs/alloy/latest/
