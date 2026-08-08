#!/usr/bin/env bash
set -euo pipefail

# Usage: ./trigger-tls.sh [expiry|handshake]
# Default to "expiry" if no argument is provided
MODE="${1:-expiry}"

case "${MODE}" in
  expiry)
    echo "==> [TLS] Mode: Testing TLSCertificateExpiringSoon (Bypassing CA validation)..."
    kubectl set env deployment/kube-prober TLS_INSECURE_SKIP_VERIFY=true
    kubectl rollout status deployment/kube-prober --timeout=60s
    DAYS_VALID=5
    ;;
  handshake)
    echo "==> [TLS] Mode: Testing TLSHandshakeFailed (Strict CA validation active)..."
    kubectl set env deployment/kube-prober TLS_INSECURE_SKIP_VERIFY=false
    kubectl rollout status deployment/kube-prober --timeout=60s
    DAYS_VALID=365
    ;;
  *)
    echo "Usage: $0 [expiry|handshake]"
    exit 1
    ;;
esac

echo "==> [TLS] Generating self-signed certificate (valid for ${DAYS_VALID} days)..."
openssl req -x509 -nodes -days "${DAYS_VALID}" -newkey rsa:2048 \
  -keyout tls.key -out tls.crt \
  -subj "/CN=tls-test.default.svc.cluster.local" 2>/dev/null

kubectl create secret tls tls-test-certs --key tls.key --cert tls.crt --dry-run=client -o yaml | kubectl apply -f -

echo "==> [TLS] Deploying TLS target service..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: tls-test-nginx-config
data:
  default.conf: |
    server {
        listen 443 ssl;
        server_name _;
        ssl_certificate /etc/nginx/certs/tls.crt;
        ssl_certificate_key /etc/nginx/certs/tls.key;
        location / {
            return 200 'OK';
            add_header Content-Type text/plain;
        }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tls-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tls-test
  template:
    metadata:
      labels:
        app: tls-test
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 443
        volumeMounts:
        - name: certs
          mountPath: /etc/nginx/certs
          readOnly: true
        - name: config
          mountPath: /etc/nginx/conf.d
          readOnly: true
      volumes:
      - name: certs
        secret:
          secretName: tls-test-certs
      - name: config
        configMap:
          name: tls-test-nginx-config
---
apiVersion: v1
kind: Service
metadata:
  name: tls-test
  labels:
    probe: "true"
  annotations:
    probe/scheme: "tls"
spec:
  ports:
  - port: 443
    targetPort: 443
    name: https
  selector:
    app: tls-test
EOF

rm tls.key tls.crt

if [ "${MODE}" = "expiry" ]; then
  echo "==> Target tls-test deployed. Alert TLSCertificateExpiringSoon will trigger shortly."
else
  echo "==> Target tls-test deployed. Alert TLSHandshakeFailed will trigger shortly."
fi