#!/usr/bin/env bash
set -euo pipefail

echo "==> [Traffic] Simulating traffic collapse by scaling httpbin to 0 replicas..."

if kubectl get deployment httpbin >/dev/null 2>&1; then
  kubectl scale deployment httpbin --replicas=0
  echo "==> httpbin scaled to 0. Probes will fail/drop, triggering TrafficCollapse alert."
else
  echo "ERROR: Deployment httpbin not found. Ensure the base helm chart is installed."
  exit 1
fi