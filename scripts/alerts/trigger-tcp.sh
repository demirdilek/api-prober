#!/usr/bin/env bash
set -euo pipefail

echo "==> [TCP] Deploying raw TCP target with intentional misconfiguration..."

kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tcp-test
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tcp-test
  template:
    metadata:
      labels:
        app: tcp-test
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: tcp-test
  namespace: default
  labels:
    probe: "true"
  annotations:
    probe/scheme: "tcp"
    probe/path: "" 
spec:
  ports:
    - port: 80
      # Intentionally pointing to a dead port to simulate a connection refused error
      targetPort: 9999
      name: tcp
  selector:
    app: tcp-test
EOF

echo "==> Target tcp-test deployed. TCPConnectionRefused alert will trigger shortly."