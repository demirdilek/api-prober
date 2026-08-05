#!/usr/bin/env bash
set -euo pipefail

echo "==> [Traffic] Simulating traffic collapse by misrouting httpbin-success service port..."

if kubectl get svc httpbin-success -n default >/dev/null 2>&1; then
  kubectl patch service httpbin-success -n default -p '{"spec":{"ports":[{"port":80,"targetPort":9999}]}}'
  echo "==> httpbin-success patched to invalid port (9999). Traffic collapse simulated."
else
  echo "ERROR: Service httpbin-success not found in default namespace."
  exit 1
fi