#!/usr/bin/env bash
set -euo pipefail

echo "==> [TCP] Deploying raw TCP target..."

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
    probe/path: "" # No path needed for raw TCP
spec:
  ports:
    - port: 80
      targetPort: 80
      name: tcp
  selector:
    app: tcp-test
EOF

echo "==> Target tcp-test deployed. Prober should now verify Layer 4 connectivity."