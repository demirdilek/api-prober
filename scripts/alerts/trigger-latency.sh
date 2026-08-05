#!/usr/bin/env bash
set -euo pipefail

echo "==> [Latency] Deploying high-latency target (/delay/2)..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-latency-test
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-latency-test
  template:
    metadata:
      labels:
        app: httpbin-latency-test
    spec:
      containers:
        - name: httpbin
          image: mccutchen/go-httpbin:latest
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin-latency-test
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/path: "/delay/2"
spec:
  ports:
    - port: 80
      targetPort: 8080
      name: http
  selector:
    app: httpbin-latency-test
EOF

echo "==> Target httpbin-latency-test deployed. Alert HighLatency will trigger shortly."