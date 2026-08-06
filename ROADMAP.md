# Production Readiness Roadmap

This document outlines the planned improvements, architectural refinements, and feature milestones for `kube-prober`.

---

## Phase 1: High Priority (Bugs & Security Hardening)

- [X] **RBAC Permissions Update (`helm/kube-prober/templates/rbac.yaml`)**
  - Add `endpointslices` under the `discovery.k8s.io` API group to prevent HTTP 403 (Forbidden) errors during Informer synchronization inside Kubernetes clusters.
- [X] **Kubernetes Probes Configuration (`helm/kube-prober/templates/deployment.yaml`)**
  - Integrate native `livenessProbe` (`/healthz`) and `readinessProbe` (`/readyz`) endpoints into the deployment spec.
- [X] **Pod Security Context Hardening (`helm/kube-prober/templates/deployment.yaml`)**
  - Enforce non-root execution (`runAsNonRoot: true`), read-only root filesystems (`readOnlyRootFilesystem: true`), and drop all unneeded Linux capabilities (`capabilities.drop: ["ALL"]`).

---

## Phase 2: Medium Priority (Features & Reliability)

- [X] **Dynamic Path Resolution via Informer (`pkg/prober/registry.go`)**
  - Parse custom Service annotations (`probe/path`) dynamically via the Informer instead of hardcoding `/healthz` in the target URL builder.
- [X] **High Availability (HA) Setup**
  - Add `PodDisruptionBudget` (PDB) and `HorizontalPodAutoscaler` (HPA) manifests to the Helm chart.
- [X] **Metrics Clean-Up on Target Deletion**
  - Unregister or clean up Prometheus metrics (Gauge/Counter labels) upon target deletion to avoid stale metrics and memory leaks.

---

## Phase 3: Low Priority & Enterprise Expansion

- [X] **Distributed Target Sharding via Consistent Hashing**
  - Implement a sharding mechanism (e.g., consistent hashing or modulo partitioning based on pod ordinal/IPs) across prober replicas when scaled via HPA to prevent duplicate probing and horizontally distribute workload.
- [ ] **SLO / SLI & Error Budget Exporting**
  - Expose calculated multi-window burn rates directly as Prometheus metrics and ship pre-configured `PrometheusRule` manifests.
- [ ] **Protocol Extension (gRPC / TCP / TLS)**
  - Extend `prober.Dispatcher` with additional protocol handlers (e.g., gRPC Health Checking, TCP banner checks, TLS certificate expiry tracking).
- [ ] **Chaos Engineering Test Suites**
  - Define Chaos Mesh or LitmusChaos scenarios to validate telemetry accuracy during simulated network latency, packet loss, and pod eviction events.
- [ ] **v1.2.0 — Multi-Zone Vantage Point Probing & Follow-the-Sun Alerting:**
  - [ ] **Locality-Aware Probing:** Inject `MY_NODE_ZONE` (via K8s Downward API / Node labels) into prober instances.
  - [ ] **Multi-Vantage Point Metrics:** Expand Prometheus metrics with `source_zone` labels (`kube_prober_latency_seconds{source_zone="..."}`) to measure global latency per region.
  - [ ] **Zonal Sharding Pools:** Filter peer lists in `watchProberPeers` by zone so each region runs its own Rendezvous Hashing ring over global targets.
  - [ ] **Follow-the-Sun Alert Routing:** Configure PrometheusRules and Alertmanager `active_time_intervals` (EU / US / APAC shifts) to route critical alerts dynamically to the active on-call team based on UTC business hours.