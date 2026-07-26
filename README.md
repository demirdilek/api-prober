[![CI](https://github.com/demirdilek/api-prober/actions/workflows/ci.yml/badge.svg](https://github.com/demirdilek/api-prober/actions/workflows/ci.yml]

# api-prober

A cloud-native, platform-independent SRE telemetry stack written in Go. This project implements and visualizes the **4 Golden Signals** (Latency, Traffic, Errors, Saturation, for distributed edge environments.

## Features

* **Dynamic Targets:** Monitored endpoints are dynamically loaded via `targets.csv`. A native background watcher applies updates on the fly, provisioning or gracefully terminating worker goroutines without requiring application restarts.
* **Multi-Stage & Multi-Arch Build:** Minimal Docker footprint supporting both `amd64` and `arm64` architectures.
* **Fully Encapsulated Stack:** Self-contained environment featuring Go, Prometheus, Alertmanager, Grafana, and Httpbin.
* **Graceful Shutdown:** Listens for termination signals (`SIGINT`, `SIGTERM`) to cleanly shut down the HTTP metric server, active workers, and dynamic target schedulers without dropping in-flight probes.
* **Configurable via Environment Variables:** Fully aligned with Twelve-Factor App principles.
* **Automated CI/CD:** GitHub Actions pipeline ensuring clean builds and dependency caching.

## Architecture

```text
api-prober Architecture
│
├── Configuration Layer
│   └── targets.csv (Dynamic target config via Helm ConfigMap)
│
├── Core Engine (Go)
│   ├── watchTargets() (Polls targets.csv every 5s)
│   ├── Dynamic Goroutines (Spawns/cancels per target via Context)
│   └── Shared http.Client (Connection pooling & 5s timeouts)
│
├── External Endpoints
│   └── Target APIs (Probed via HTTP GET)
│
└── Observability & Diagnostics Stack
    ├── Structured Logs (JSON stdout via slog)
    ├── Go HTTP Exporter (:8080) ──> Serves /metrics & /debug/pprof
    ├── Prometheus ─────────> Scrapes /metrics every interval via ServiceMonitor
    └── Grafana ──────────> Queries Prometheus via PromQL (Auto-provisioned dashboard)
```

## Getting Started

### Prerequisites

* Docker
* k3d / Kubernetes
* Helm
* GNU Make
* 1Password CLI (op) for secret management

### Local Development & Testing

Run unit and integration tests with the race detector enabled:

```bash
go test -v -race ./...
```

### Local Cluster Lifecycle & Helm Deployment

You can manage the local development cluster and the Helm chart lifecycle using the provided `Makefile`:

```bash
# 1. Create a local k3d Kubernetes cluster
make k3d-up

# 2. Build the local Docker image and import it into k3d
make docker-build
k3d image import ghcr.io/demirdilek/api-prober:latest -c mycluster

# 3. Install the Prometheus monitoring stack (if not already present)
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prom-stack prometheus-community/kube-prometheus-stack

# 4. Install the application into the cluster
make helm-install
```

## Kubernetes & Helm Configuration

The application runs inside a Kubernetes cluster managed via k3d, paired with the kube-prometheus-stack Helm chart.

* Chart Location: ./helm/api-prober

* Secret Management: Sensitive credentials—such as the Pushover token and user_key—are pulled securely at runtime from 1Password using the 1Password CLI (op read) and injected directly into Kubernetes Secrets.

```bash
kubectl create secret generic pushover-credentials \
  --from-literal=user_key="$(op read 'op://vault/item/user_key')" \
  --from-literal=token="$(op read 'op://vault/item/api_token')"
```

* Observability Stack: Includes Prometheus for scraping metrics via ServiceMonitor, Alertmanager with AlertmanagerConfig for Pushover notifications, and sidecar-provisioned Grafana dashboards.

## Alerting & Escalation

Real-time notifications are managed via Prometheus Alertmanager based on thresholds of the 4 Golden Signals.

### Emergency Priority (iOS Silent Mode Bypass)
Alerts are pre-configured with Priority 2 (Emergency). To ensure critical operational alerts break through your phone's silent switch or "Do Not Disturb" focus modes:

* 1. Open Settings on your iOS device.
* 2. Navigate to Pushover -> Notifications.
* 3. Enable Allow Critical Alerts.

### Simulating an Alert
To verify the entire alerting pipeline from the edge to your phone, add a failing target to targets.csv:

Code-Snippet
```text
http://httpbin/status/500
```