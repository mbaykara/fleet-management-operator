# Fleet Management Operator

A Kubernetes operator for managing [Grafana Cloud Fleet Management](https://grafana.com/docs/grafana-cloud/send-data/fleet-management/) Pipelines as native Kubernetes resources.

## Overview

This operator enables declarative management of Fleet Management configuration pipelines using Kubernetes. Define your Alloy or OpenTelemetry Collector configurations as Kubernetes resources, and the operator automatically syncs them to Grafana Cloud Fleet Management.

### Features

- **Declarative Pipeline Management**: Define pipelines as Kubernetes resources
- **Dual Config Support**: Both Grafana Alloy and OpenTelemetry Collector configurations
- **Source Tracking**: Track pipeline origins (Git, Terraform, Kubernetes)
- **Multi-Architecture Support**: Docker images for linux/amd64 and linux/arm64
- **GitOps Friendly**: Manage pipelines through version control
- **Status Tracking**: Pipeline status reflects Fleet Management state with conditions
- **Finalizer Protection**: Proper cleanup when pipelines are deleted
- **Helm Chart**: Easy installation and configuration
- **Leader Election**: High availability support with multiple replicas

## Prerequisites

- Kubernetes v1.32.3+ cluster
- Helm 4.0+ (for Helm installation)
- **Grafana Cloud Fleet Management credentials** (base URL, username, password/token)

## Quick Start

### 1. Set Fleet Management Credentials

```bash
export FLEET_MANAGEMENT_BASE_URL="https://fleet-management-<CLUSTER>.grafana.net/pipeline.v1.PipelineService/"
export FLEET_MANAGEMENT_USERNAME="your-username"
export FLEET_MANAGEMENT_PASSWORD="your-password-or-token"
#or
kubectl create secret generic fleet-management-credentials \
  -n fleet-management-system \
  --from-literal=base-url='https://fleet-management-prod-001.grafana.net/pipeline.v1.PipelineService/' \
  --from-literal=username='12345' \
  --from-literal=password='glc_xxxxxxxxxxxxx'
```

Get these from your Grafana Cloud Fleet Management interface:
- Navigate to **Connections > Collector > Fleet Management**
- Switch to the **API tab**
- Find the base URL and credentials

### 2. Install CRDs

```bash
make install
```

### 3. Run the Controller Locally

```bash
make run
```

### 4. Create a Pipeline

In another terminal:

```bash
kubectl apply -f config/samples/fleetmanagement_v1alpha1_pipeline.yaml
```

### 5. Check Status

```bash
# List all pipelines
kubectl get pipelines
# or use the short name
kubectl get fmp

# Get detailed status
kubectl describe pipeline pipeline-sample

# Watch reconciliation
kubectl get pipelines -w
```

## Pipeline Resource Examples

### Grafana Alloy Configuration

```yaml
apiVersion: fleetmanagement.grafana.com/v1alpha1
kind: Pipeline
metadata:
  name: prometheus-metrics
spec:
  # Alloy configuration
  contents: |
    prometheus.exporter.self "alloy" { }

    prometheus.scrape "alloy" {
      targets    = prometheus.exporter.self.alloy.targets
      forward_to = [prometheus.remote_write.grafanacloud.receiver]
      scrape_interval = "60s"
    }

    prometheus.remote_write "grafanacloud" {
      external_labels = {"collector_id" = constants.hostname}
      endpoint {
        url = env("PROMETHEUS_URL")
        basic_auth {
          username      = env("PROMETHEUS_USER")
          password_file = "/etc/secrets/prometheus-password"
        }
      }
    }

  # Assign to collectors with these attributes
  matchers:
    - collector.os=linux
    - environment=production

  enabled: true
  configType: Alloy  # or OpenTelemetryCollector
```

### OpenTelemetry Collector Configuration

```yaml
apiVersion: fleetmanagement.grafana.com/v1alpha1
kind: Pipeline
metadata:
  name: otel-metrics
spec:
  # OpenTelemetry Collector YAML configuration
  contents: |
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317

    processors:
      batch:
        timeout: 10s

    exporters:
      prometheusremotewrite:
        endpoint: ${env:PROMETHEUS_URL}

    service:
      pipelines:
        metrics:
          receivers: [otlp]
          processors: [batch]
          exporters: [prometheusremotewrite]

  matchers:
    - collector.type=otel
    - environment=production

  enabled: true
  configType: OpenTelemetryCollector  # Important!
```

## Pipeline Status

The controller updates the Pipeline status to reflect its state in Fleet Management:

```yaml
status:
  id: "12345"                    # Fleet Management pipeline ID
  observedGeneration: 1          # Last reconciled generation
  createdAt: "2024-01-15T10:00:00Z"
  updatedAt: "2024-01-15T10:00:00Z"
  revisionId: "67890"            # Current revision
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: Pipeline successfully synced to Fleet Management
  - type: Synced
    status: "True"
    reason: UpsertSucceeded
```

## Deployment to Cluster

### Build and Push Image

```bash
make docker-build docker-push IMG=<your-registry>/fleet-management-operator:v1.0.0
```

### Create Credentials Secret

```bash
kubectl create namespace fleet-management-operator-system

kubectl create secret generic fleet-management-credentials \
  -n fleet-management-operator-system \
  --from-literal=FLEET_MANAGEMENT_BASE_URL="https://fleet-management-prod-001.grafana.net/pipeline.v1.PipelineService/" \
  --from-literal=FLEET_MANAGEMENT_USERNAME="your-username" \
  --from-literal=FLEET_MANAGEMENT_PASSWORD="your-password"
```

### Deploy Controller

```bash
make deploy IMG=<your-registry>/fleet-management-operator:v1.0.0
```

Update `config/manager/manager.yaml` to reference the secret:

```yaml
env:
  - name: FLEET_MANAGEMENT_BASE_URL
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: FLEET_MANAGEMENT_BASE_URL
  - name: FLEET_MANAGEMENT_USERNAME
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: FLEET_MANAGEMENT_USERNAME
  - name: FLEET_MANAGEMENT_PASSWORD
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: FLEET_MANAGEMENT_PASSWORD
```

## Configuration

The controller requires three environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `FLEET_MANAGEMENT_BASE_URL` | Fleet Management Pipeline API endpoint | `https://fleet-management-prod-001.grafana.net/pipeline.v1.PipelineService/` |
| `FLEET_MANAGEMENT_USERNAME` | API username | `12345` |
| `FLEET_MANAGEMENT_PASSWORD` | API password or token | `glc_xxxxx` |

## Development

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -v ./internal/controller -run TestControllers
```

### Project Structure

```
├── api/v1alpha1/              # CRD definitions
│   └── pipeline_types.go
├── internal/controller/       # Controller implementation
│   ├── pipeline_controller.go
│   └── pipeline_controller_test.go
├── pkg/fleetclient/          # Fleet Management API client
│   ├── client.go
│   └── types.go
├── config/
│   ├── crd/bases/            # Generated CRD manifests
│   ├── samples/              # Example pipelines
│   └── rbac/                 # RBAC configuration
└── cmd/main.go               # Controller entry point
```

### Key Files

- **CLAUDE.md**: Comprehensive development guide with Fleet Management API details, controller patterns, and Go best practices
- **IMPLEMENTATION_STATUS.md**: Detailed implementation checklist and features
- **config/samples/**: Example Pipeline resources for both Alloy and OTEL

## Troubleshooting

### Pipeline not syncing

Check the controller logs:

```bash
kubectl logs -f deployment/fleet-management-operator-controller-manager -n fleet-management-operator-system
```

Check Pipeline status:

```bash
kubectl describe pipeline <pipeline-name>
```

Look for condition messages in the status.

### Common Issues

1. **Pipeline validation error**: Check `status.conditions` for validation errors from Fleet Management API
2. **Rate limit exceeded**: Controller automatically retries with exponential backoff
3. **Credentials invalid**: Verify `FLEET_MANAGEMENT_*` environment variables are correct
4. **Pipeline stuck in Terminating**: Check controller logs for finalizer removal errors

## Advanced Usage

### Using spec.name vs metadata.name

By default, the controller uses `metadata.name` as the pipeline name in Fleet Management. Override this with `spec.name`:

```yaml
metadata:
  name: k8s-pipeline-name        # Kubernetes resource name
spec:
  name: fleet-management-name    # Fleet Management pipeline name
```

### ConfigType Validation

The controller validates that `configType` matches the configuration syntax:
- `Alloy`: For Alloy configuration (default)
- `OpenTelemetryCollector`: For OTEL YAML configuration

Mismatched types will cause validation errors.

### Matcher Syntax

Matchers follow Prometheus Alertmanager syntax:
- `key=value` - Equals
- `key!=value` - Not equals
- `key=~regex` - Regex match
- `key!~regex` - Regex not match

Maximum 200 characters per matcher.

## API Reference

See **CLAUDE.md** for complete Fleet Management API documentation including:
- All API operations (UpsertPipeline, DeletePipeline, etc.)
- Request/response formats
- Error handling
- Rate limits
- Revision tracking

## Contributing

This project follows Kubernetes best practices and Go conventions. See **CLAUDE.md** for:
- Controller development patterns
- Kubernetes controller pitfalls to avoid
- Go best practices
- Testing strategies

## Uninstall

```bash
# Delete sample pipelines
kubectl delete -f config/samples/

# Uninstall CRDs
make uninstall

# Undeploy controller
make undeploy
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Resources

- [Fleet Management Documentation](https://grafana.com/docs/grafana-cloud/send-data/fleet-management/)
- [Kubebuilder Documentation](https://book.kubebuilder.io/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
