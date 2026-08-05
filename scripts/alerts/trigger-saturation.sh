#!/usr/bin/env bash
set -euo pipefail

echo "==> [Saturation] Deploying dual slow targets & setting WORKERS=2 to fully saturate worker pool..."

# 1. Deployment von zwei langlaufenden Targets (/delay/2)
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-sat-1
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-sat-1
  template:
    metadata:
      labels:
        app: httpbin-sat-1
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
  name: httpbin-sat-1
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
    app: httpbin-sat-1
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin-sat-2
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin-sat-2
  template:
    metadata:
      labels:
        app: httpbin-sat-2
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
  name: httpbin-sat-2
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
    app: httpbin-sat-2
EOF

# 2. Worker-Kapazität im kube-prober auf 2 drosseln
if kubectl get deployment kube-prober >/dev/null 2>&1; then
  kubectl set env deployment/kube-prober WORKERS=2
  echo "==> WORKERS capacity reduced to 2 with 2 concurrent slow targets. Alert MaxWorkersReached will trigger in ~1 min."
else
  echo "ERROR: Deployment kube-prober not found."
  exit 1
fi