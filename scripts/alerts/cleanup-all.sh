#!/usr/bin/env bash
set -euo pipefail

echo "==> Cleaning up all simulated alert targets & restoring defaults..."

# 1. Alle künstlichen Test-Deployments löschen
kubectl delete deployment httpbin-error httpbin-latency-test httpbin-sat-1 httpbin-sat-2 --ignore-not-found
kubectl delete service httpbin-error httpbin-latency-test httpbin-sat-1 httpbin-sat-2 --ignore-not-found
kubectl delete deployment httpbin-error httpbin-latency-test httpbin-sat-1 httpbin-sat-2 tcp-test --ignore-not-found
kubectl delete service httpbin-error httpbin-latency-test httpbin-sat-1 httpbin-sat-2 tcp-test --ignore-not-found
kubectl delete deployment tls-test --ignore-not-found
kubectl delete service tls-test --ignore-not-found
kubectl delete secret tls-test-certs --ignore-not-found
kubectl delete configmap tls-test-nginx-config --ignore-not-found

# 2. Basis-Service Port wiederherstellen (Port 80 -> 8080)
kubectl patch service httpbin-success -n default --type=merge -p '{"spec":{"ports":[{"port":80,"targetPort":8080}]}}' 2>/dev/null || true

# 3. WORKERS und Replicas auf Standard zurücksetzen
if kubectl get deployment kube-prober >/dev/null 2>&1; then
  kubectl set env deployment/kube-prober WORKERS=50
fi

if kubectl get deployment httpbin >/dev/null 2>&1; then
  kubectl scale deployment httpbin --replicas=1
fi

# Set the Insecure verifieng to false 
kubectl set env deployment/kube-prober TLS_INSECURE_SKIP_VERIFY=false

echo "==> Waiting 5 seconds for EndpointSlices to settle..."
sleep 5

echo "==> Restarting kube-prober to flush old target metrics..."
# NUR kube-prober neustarten, damit er frische EndpointSlices liest und In-Memory-Metriken leert
kubectl rollout restart deployment kube-prober 2>/dev/null || true

echo "==> Cleanup complete!"