#!/usr/bin/env bash
set -euo pipefail

echo "==> [Saturation] Temporarily setting WORKERS=2 to saturate worker pool..."

if kubectl get deployment kube-prober >/dev/null 2>&1; then
  kubectl set env deployment/kube-prober WORKERS=2
  echo "==> WORKERS capacity reduced to 2. Alert MaxWorkersReached will trigger shortly."
else
  echo "ERROR: Deployment kube-prober not found."
  exit 1
fi