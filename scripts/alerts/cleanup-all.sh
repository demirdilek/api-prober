#!/usr/bin/env bash
set -euo pipefail

echo "==> Cleaning up all simulated alert targets & restoring defaults..."

# Delete artificial test deployments & services
kubectl delete deployment httpbin-error httpbin-latency-test --ignore-not-found
kubectl delete service httpbin-error httpbin-latency-test --ignore-not-found

# Restore base httpbin replicas to 1
if kubectl get deployment httpbin >/dev/null 2>&1; then
  kubectl scale deployment httpbin --replicas=1
fi

# Restore default WORKERS capacity to 50
if kubectl get deployment kube-prober >/dev/null 2>&1; then
  kubectl set env deployment/kube-prober WORKERS=50
fi

echo "==> Cleanup complete! Cluster restored to default state."

# Set the port to normal state(:8080). It is for the trigger-traffic.sh script
kubectl patch service httpbin-success -n default --type=merge -p '{"spec":{"ports":[{"port":80,"targetPort":80}]}}' 2>/dev/null || true