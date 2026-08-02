# api-prober

[![CI](https://github.com/demirdilek/api-prober/actions/workflows/ci.yml/badge.svg)](https://github.com/demirdilek/api-prober/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/demirdilek/api-prober?color=00ADD8&logo=go)](https://github.com/demirdilek/api-prober)
[![Image Size](https://img.shields.io/badge/image%20size-15%20MB-blue?logo=docker)](https://github.com/demirdilek/api-prober/pkgs/container/api-prober)

A cloud-native, platform-independent SRE telemetry stack written in Go. This project implements and visualizes the **4 Golden Signals** (Latency, Traffic, Errors, Saturation) for distributed Kubernetes environments.

---

## 📦 Container Image Specs

- **Registry:** `ghcr.io/demirdilek/api-prober:latest`
- **Base Image:** `gcr.io/distroless/static`
- **Architecture:** Multi-Arch (`amd64` / `arm64`)

---

## Key Features

- **Dynamic K8s Targets:** Monitored endpoints are dynamically discovered via Kubernetes API labels (`probe=true`) and custom path annotations (`probe/path="/healthz"`).
- **6-Tier SRE Error Classification:** Categorizes failures into discrete buckets: `dns_error`, `connection_refused`, `tls_error`, `timeout`, `http_error` (4xx/5xx), and `unknown_error`.
- **Actionable Diagnostic Hints:** Enriches structured `slog` JSON outputs with direct troubleshooting hints (`hint`) to lower Mean Time To Recovery (MTTR).
- **GitOps Continuous Delivery:** Fully automated deployment, sync, and self-healing managed declaratively via Argo CD.
- **Declarative Telemetry Stack:** Pre-configured Prometheus monitoring, Grafana sidecars, and Alertmanager routing via `prom-stack-values.yaml`.
- **Graceful Shutdown:** Listens for termination signals (`SIGINT`, `SIGTERM`) to cleanly shut down without dropping in-flight probes.

---

## Tech Stack & Architecture

- **Go (Golang):** Core microservice architecture featuring native API probing and Prometheus instrumentation.
- **Kubernetes:** Dynamic service discovery utilizing Kubernetes Informers.
- **Argo CD:** GitOps controller executing continuous delivery and automated cluster state synchronization.
- **Prometheus & Grafana:** Full observability stack measuring the 4 Golden Signals.
- **Alertmanager & Pushover:** Real-time mobile alert notifications triggered by defined threshold violations.

---

## Architecture Overview

The `api-prober` microservice acts as the central observability engine, continuously monitoring the Kubernetes API for discoverable services and exporting comprehensive 4 Golden Signals telemetry.

```text
[ K8s API Server ] --(Informer)--> [ api-prober ] --(HTTP/DNS Probes)--> [ Target Services ]
                                     |
                           (Prometheus Metrics)
                                     v
                               [ Prometheus ]
                                     |
                                 (Alert Rules)
                                     v
                               [ Alertmanager ]
                                     |
                                 (Pushover API)
                                     v
                               [ SRE On-Call ]
```

---

## Project Structure

```text
.
├── .github/
│   └── workflows/          # GitHub Actions CI & Docker Build Pipelines
├── assets/                 # Documentation Screenshots
├── deploy/
│   └── argocd/             # Argo CD Application Manifests (GitOps)
├── helm/
│   └── api-prober/         # Custom Helm Chart (Deployment, RBAC, ServiceMonitor, Dashboard)
│       ├── dashboards/     # Auto-provisioned Grafana Dashboards
│       └── templates/      # K8s Resources & Alerting Rules
├── Dockerfile              # Multi-stage, Multi-arch Build File
├── Makefile                # Complete Lifecycle Automation (k3d, Argo CD, Helm)
├── main.go                 # Core Probing Engine & Dynamic K8s Service Watcher
├── main_test.go            # Unit and Integration Tests
└── prom-stack-values.yaml  # Prometheus Stack & Alertmanager Routing Config
```

---

## Getting Started

### Prerequisites

- Docker / Buildx
- k3d / Kubernetes
- Helm 3+
- GNU Make

---

## Local Cluster Lifecycle & Deployment

You can manage the local development cluster and the entire stack lifecycle using the provided `Makefile`:

```bash
# Spin up the entire stack from scratch (k3d, Docker build, Prometheus, Argo CD, Helm deployment)
make all

# Fast local rebuild, import, pause GitOps auto-sync, and rollout restart for local debugging
make local-deploy

# Pause Argo CD Auto-Sync & Self-Healing for local debugging
make dev-enable

# Re-enable Argo CD Auto-Sync & Self-Healing
make dev-disable

# Delete local k3d cluster and clean up local artifacts
make clean

# Start background port-forwarding for Argo CD (8080), Prometheus (9090), and Grafana (3000)
make forward-all

# Stop background port-forwarding
make stop-forward

# Run unit and integration tests with the race detector enabled
make test
```

![Kubernetes Pods](assets/pods.png)

---

## Observability & Dashboards

The Grafana dashboard is **fully auto-provisioned out of the box** via the Prometheus Operator sidecar mechanism (`grafana_dashboard: "1"`), requiring zero manual JSON imports or configuration. It visualizes real-time telemetry for all 4 Golden Signals: Latency, Traffic, Errors, and Saturation.

Once background port-forwarding is active (`make forward-all`), access the Control Plane UIs via your browser or Tailscale network:

- **Argo CD:** [https://localhost:8080](https://localhost:8080) or [https://<TAILSCALE_IP>:8080](https://<TAILSCALE_IP>:8080)
- **Prometheus:** [http://localhost:9090](http://localhost:9090) or [http://<TAILSCALE_IP>:9090](http://<TAILSCALE_IP>:9090)
- **Grafana:** [http://localhost:3000](http://localhost:3000) or [http://<TAILSCALE_IP>:3000](http://<TAILSCALE_IP>:3000)

| Argo CD Control Plane | Prometheus Target Telemetry |
| :---: | :---: |
| ![Argo](assets/argo.png) | ![Prometheus](assets/prometheus.png) |

| Grafana 4 Golden Signals Dashboard | Structured Slog Output |
|f :---: | :---: |
| ![Grafana Dashboard](assets/grafana-dashboard.png) | ![Slog Output](assets/slog-output.png) |

---

## Alerting & Escalation

Real-time notifications are managed via Prometheus Alertmanager based on thresholds of the 4 Golden Signals.

### Emergency Priority (iOS Silent Mode Bypass)

Alerts are pre-configured to escalate critical issues. To ensure critical operational alerts break through your phone's silent switch or "Do Not Disturb" focus modes:

1. Open **Settings** on your iOS device.
2. Navigate to **Pushover** -> **Notifications**.
3. Enable **Allow Critical Alerts**.

| Latency Threshold Alert | High Error Rate Escalation |
| :---: | :---: |
| ![Pushover 1](assets/pushover1.png) | ![Pushover 2](assets/pushover2.png) |

---

## Simulating Alerts

The setup allows you to simulate alerts for different Golden Signals:

### 1. High Latency Alert

The Helm chart automatically provisions a target called `httpbin-slow` which simulates delayed responses (`/delay/1`). Once Prometheus evaluates the high p99 latency threshold over the defined timeframe, Alertmanager will push a notification to your configured Pushover device.

### 2. High Error Rate Alert (Dynamic Discovery & Error Target Simulation)

You can trigger a real-time HTTP 500 error alert using the Makefile:

```bash
# Deploy an artificial error target (HTTP 500)
make test-alert

# Clean up the error target after testing
make test-alert-clean
```

The Go engine will dynamically pick up the new endpoint within 5 seconds and start probing it. Shortly after, the `HighErrorRate` rule will fire in Prometheus and escalate to Alertmanager.

---

## License

[MIT License](https://opensource.org/licenses/MIT)
