-include .env
export

.PHONY: help k3d-up docker-build clean-build prometheus-install helm-install all helm-upgrade helm-uninstall local-deploy hard-reset k3d-down clean forward-all stop-forward test lint test-coverage install-argocd apply-gitops argocd-pass create-secrets test-alert test-alert-clean dev-enable dev-disable

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO=ghcr.io/demirdilek/api-prober
IMAGE_TAG=dev

ARGOCD_MANIFEST_URL ?= https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
TAILSCALE_IP ?= $(shell tailscale ip -4 2>/dev/null || echo "localhost")

# Helm variables
RELEASE_NAME=api-prober
CHART_DIR=./helm/api-prober

# Argo CD variables
ARGO_APP ?= api-prober
ARGO_NAMESPACE ?= argocd

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

lint: ## Run golangci-lint or go vet for code quality
	@echo "==> Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet..."; \
		go vet ./...; \
	fi

test: ## Run unit and integration tests with race detection
	@echo "==> Running tests with race detector..."
	go test -v -race ./...

test-coverage: ## Run tests and generate HTML coverage report
	@echo "==> Generating test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

k3d-up: ## 1. Create a local k3d Kubernetes cluster
	@if k3d cluster list | grep -q "mycluster"; then \
		echo "Cluster 'mycluster' already exists."; \
	else \
		k3d cluster create mycluster --api-port 6443 -p "80:80@loadbalancer" -p "443:443@loadbalancer" --agents 2; \
	fi

docker-build: lint test ## 2. Build local Docker image and import into k3d (runs lint & test first)
	@echo "==> Building Docker image..."
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	@echo "==> Importing image into k3d..."
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster

clean-build: ## Force a clean build by wiping BuildKit cache
	docker builder prune --all -f
	docker build --no-cache -t $(IMAGE_REPO):$(IMAGE_TAG) .

prometheus-install: ## 3. Install kube-prometheus-stack via Helm
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update
	@if ! helm status prom-stack > /dev/null 2>&1; then \
		helm install prom-stack prometheus-community/kube-prometheus-stack -f prom-stack-values.yaml; \
	else \
		echo "Prometheus stack already installed."; \
	fi

install-argocd: ## 4. Install Argo CD components into the cluster
	@echo "==> Installing Argo CD..."
	kubectl create namespace argocd || true
	kubectl apply -n argocd --server-side --force-conflicts -f $(ARGOCD_MANIFEST_URL)
	@echo "==> Waiting for Argo CD components to be ready..."
	kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=300s

apply-gitops: ## 5. Register the api-prober application in Argo CD
	@echo "==> Registering api-prober Application in Argo CD..."
	kubectl apply -f deploy/argocd/api-prober-app.yaml

helm-install: ## 6. Deploy application Helm chart (api-prober)
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR)

dev-enable: ## Pause Argo CD Auto-Sync & Self-Healing for local debugging
	@echo "==> Disabling Argo CD Auto-Sync for $(ARGO_APP)..."
	kubectl patch application $(ARGO_APP) -n $(ARGO_NAMESPACE) --type merge -p '{"spec":{"syncPolicy":{"automated":null}}}'

dev-disable: ## Re-enable Argo CD Auto-Sync & Self-Healing
	@echo "==> Re-enabling Argo CD Auto-Sync for $(ARGO_APP)..."
	kubectl patch application $(ARGO_APP) -n $(ARGO_NAMESPACE) --type merge -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'

local-deploy: dev-enable lint test docker-build create-secrets ## Fast local rebuild, import, pause GitOps, and rollout restart
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	kubectl rollout restart deployment $(RELEASE_NAME)

all: k3d-up create-secrets docker-build prometheus-install install-argocd apply-gitops helm-install ## Bootstrap entire local stack out-of-the-box
	@echo "========================================================="
	@echo " api-prober stack is fully up and running out-of-the-box! "
	@echo "========================================================="

helm-upgrade: ## Upgrade existing api-prober Helm release
	helm upgrade $(RELEASE_NAME) $(CHART_DIR)

helm-uninstall: ## Remove api-prober Helm release
	helm uninstall $(RELEASE_NAME) || true

k3d-down: ## Delete local k3d cluster
	k3d cluster delete mycluster || true

clean: k3d-down ## Clean up cluster and temporary build files
	rm -f project-dump.txt coverage.out coverage.html .argo.pid .prom.pid .grafana.pid

forward-all: ## Forward Argo CD, Prometheus & Grafana UIs for Mobile/Tailscale
	@echo "========================================================"
	@echo " CONTROL PLANE WEB UIs (Tailscale / iPhone Access)"
	@echo "========================================================"
	@echo " ARGO CD:    https://$(TAILSCALE_IP):8080"
	@echo "   User:     admin"
	@echo -n "   Password: "
	@kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" | base64 -d 2>/dev/null || echo "Not found"
	@echo " "
	@echo "--------------------------------------------------------"
	@echo " PROMETHEUS: http://$(TAILSCALE_IP):9090"
	@echo "--------------------------------------------------------"
	@echo " GRAFANA:    http://$(TAILSCALE_IP):3000"
	@echo "   User:     admin"
	@echo -n "   Password: "
	@kubectl -n default get secret prom-stack-grafana -o jsonpath="{.data.admin-password}" | base64 -d 2>/dev/null || echo "admin"
	@echo " "
	@echo "========================================================"
	@echo "==> Starting Port-Forwards in background..."
	@kubectl port-forward --address 0.0.0.0 -n argocd svc/argocd-server 8080:443 >/dev/null 2>&1 & echo $$! > .argo.pid
	@kubectl port-forward --address 0.0.0.0 -n default svc/prom-stack-kube-prometheus-prometheus 9090:9090 >/dev/null 2>&1 & echo $$! > .prom.pid
	@kubectl port-forward --address 0.0.0.0 -n default svc/prom-stack-grafana 3000:80 >/dev/null 2>&1 & echo $$! > .grafana.pid
	@echo "==> Done! All 3 UIs are accessible via Tailscale."

stop-forward: ## Stop background port-forwarding
	@pkill -f "kubectl port-forward" 2>/dev/null || true
	@rm -f .argo.pid .prom.pid .grafana.pid
	@echo "Stopped all port-forwards."

argocd-pass: ## Retrieve initial admin password for Argo CD UI
	@echo "==> Argo CD Initial Admin Password:"
	@kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" | base64 -d; echo ""

create-secrets: ## Create Kubernetes secret from local .env credentials
	@echo "==> Creating pushover-credentials secret in k3d..."
	@kubectl create secret generic pushover-credentials \
		--from-literal=USER_KEY=$(PUSHOVER_USER_KEY) \
		--from-literal=APP_TOKEN=$(PUSHOVER_API_TOKEN) \
		--dry-run=client -o yaml | kubectl apply -f -

hard-reset: clean all ## Deep clean cluster and rebuild stack fresh

test-alert: ## Test target to simulate an HTTP 500 error for Pushover alerts
	@echo "==> Deploying error target (HTTP 500)..."
	@printf '%s\n' \
		'apiVersion: apps/v1' \
		'kind: Deployment' \
		'metadata:' \
		'  name: httpbin-error' \
		'  namespace: default' \
		'spec:' \
		'  replicas: 1' \
		'  selector:' \
		'    matchLabels:' \
		'      app: httpbin-error' \
		'  template:' \
		'    metadata:' \
		'      labels:' \
		'        app: httpbin-error' \
		'    spec:' \
		'      containers:' \
		'        - name: httpbin' \
		'          image: mccutchen/go-httpbin:latest' \
		'          ports:' \
		'            - containerPort: 8080' \
		'---' \
		'apiVersion: v1' \
		'kind: Service' \
		'metadata:' \
		'  name: httpbin-error' \
		'  namespace: default' \
		'  labels:' \
		'    probe: "true"' \
		'  annotations:' \
		'    probe/path: "/status/500"' \
		'spec:' \
		'  ports:' \
		'    - port: 80' \
		'      targetPort: 8080' \
		'      name: http' \
		'  selector:' \
		'    app: httpbin-error' | kubectl apply -f -

test-alert-clean: ## Cleanup target for the error simulation
	@echo "==> Cleaning up error target..."
	@kubectl delete deployment httpbin-error --ignore-not-found
	@kubectl delete service httpbin-error --ignore-not-found