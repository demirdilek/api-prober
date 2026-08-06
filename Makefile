-include .env
export

.PHONY: help k3d-up docker-build clean-build prometheus-install helm-install all helm-upgrade helm-uninstall helm-install-prod local-deploy local-deploy-clean hard-reset k3d-down clean forward-all stop-forward test lint test-coverage install-argocd apply-gitops argocd-pass test-targets-enable test-targets-disable trigger-slow-alert test-alert-error test-alert-latency test-alert-traffic test-alert-saturation test-alert-clean dev-enable dev-disable dev-status

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO=ghcr.io/demirdilek/kube-prober
IMAGE_TAG=dev

ARGOCD_MANIFEST_URL ?= https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
TAILSCALE_IP ?= $(shell tailscale ip -4 2>/dev/null || echo "localhost")

# Helm variables
RELEASE_NAME=kube-prober
CHART_DIR=./helm/kube-prober

# Argo CD variables
ARGO_APP ?= kube-prober
ARGO_NAMESPACE ?= argocd

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

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

prometheus-install: ## Install/upgrade kube-prometheus-stack via Helm (supports local override & .env secrets)
	@./scripts/deploy-prometheus.sh

install-argocd: ## 4. Install Argo CD components into the cluster
	@echo "==> Installing Argo CD..."
	kubectl create namespace argocd || true
	kubectl apply -n argocd --server-side --force-conflicts -f $(ARGOCD_MANIFEST_URL)
	@echo "==> Waiting for Argo CD components to be ready..."
	kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=300s

apply-gitops: ## 5. Register the kube-prober application in Argo CD
	@echo "==> Registering kube-prober Application in Argo CD..."
	kubectl apply -f deploy/argocd/kube-prober-app.yaml

helm-install: ## 6. Deploy application Helm chart (kube-prober)
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR)

dev-enable: ## Pause Argo CD Auto-Sync & Self-Healing for local debugging
	@echo "==> Disabling Argo CD Auto-Sync for $(ARGO_APP)..."
	kubectl patch application $(ARGO_APP) -n $(ARGO_NAMESPACE) --type merge -p '{"spec":{"syncPolicy":{"automated":null}}}'

dev-disable: ## Re-enable Argo CD Auto-Sync & Self-Healing
	@echo "==> Re-enabling Argo CD Auto-Sync for $(ARGO_APP)..."
	kubectl patch application $(ARGO_APP) -n $(ARGO_NAMESPACE) --type merge -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'

dev-status: ## Check if Argo CD Auto-Sync is currently enabled or disabled
	@kubectl get application $(ARGO_APP) -n $(ARGO_NAMESPACE) -o jsonpath='{"Auto-Sync status: "}{.spec.syncPolicy.automated}{"\n"}'

local-deploy: dev-enable lint test docker-build ## Fast local rebuild, import, pause GitOps, and rollout restart
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	helm template $(RELEASE_NAME) $(CHART_DIR) | kubectl apply -f -
	kubectl rollout restart deployment $(RELEASE_NAME)

local-deploy-clean: dev-enable clean-build ## Force clean build, import, and rollout restart without cache
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	helm template $(RELEASE_NAME) $(CHART_DIR) | kubectl apply -f -
	kubectl rollout restart deployment $(RELEASE_NAME)

all: k3d-up prometheus-install install-argocd apply-gitops helm-install ## Bootstrap entire local stack out-of-the-box (GitOps managed)
	@echo "========================================================="
	@echo " kube-prober stack is fully up and running out-of-the-box! "
	@echo "========================================================="

helm-upgrade: ## Upgrade existing kube-prober Helm release
	helm template $(RELEASE_NAME) $(CHART_DIR) | kubectl apply -f -

helm-uninstall: ## Remove kube-prober Helm release
	helm uninstall $(RELEASE_NAME) || true

helm-install-prod: ## Deploy Helm chart with Production overrides (no httpbin)
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR) -f $(CHART_DIR)/values-prod.yaml

k3d-down: ## Delete local k3d cluster
	k3d cluster delete mycluster || true

clean: k3d-down ## Clean up cluster and temporary build files
	rm -f coverage.out coverage.html .argo.pid .prom.pid .grafana.pid

forward-all: ## Forward Argo CD, Prometheus & Grafana UIs for Mobile/Tailscale
	@./scripts/forward-all.sh

stop-forward: ## Stop background port-forwarding
	@pkill -f "kubectl port-forward" 2>/dev/null || true
	@rm -f .argo.pid .prom.pid .grafana.pid
	@echo "Stopped all port-forwards."

argocd-pass: ## Retrieve initial admin password for Argo CD UI
	@echo "==> Argo CD Initial Admin Password:"
	@kubectl -n argocd get secret argocd-initialadmin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "Initial secret deleted. Use custom patched password or check argocd-secret." ; echo""

hard-reset: clean all ## Deep clean cluster and rebuild stack fresh

test-targets-enable: ## Scale up test targets to simulate traffic and latency
	@echo "Enabling test targets (httpbin-slow, httpbin)..."
	kubectl scale deployment httpbin-slow --replicas=1 -n default 2>/dev/null || true
	kubectl scale deployment httpbin --replicas=1 -n default 2>/dev/null || true

test-targets-disable: ## Scale down test targets to 0 replicas (clean baseline)
	@echo "Disabling test targets to avoid resource usage..."
	kubectl scale deployment httpbin-slow --replicas=0 -n default 2>/dev/null || true
	kubectl scale deployment httpbin --replicas=0 -n default 2>/dev/null || true

trigger-slow-alert: test-targets-enable ## Scale up slow endpoint to trigger HighLatency alert
	@echo "httpbin-slow enabled. HighLatency alert should fire within ~2 minutes."

test-alert-error: ## Simulate High Error Rate (HTTP 500)
	@./scripts/alerts/trigger-error.sh

test-alert-latency: ## Simulate High Latency (/delay/2)
	@./scripts/alerts/trigger-latency.sh

test-alert-traffic: ## Simulate Traffic Collapse (scale to 0)
	@./scripts/alerts/trigger-traffic.sh

test-alert-saturation: ## Simulate Worker Capacity Saturation (WORKERS=2)
	@./scripts/alerts/trigger-saturation.sh

test-alert-clean: ## Clean up all simulated alert targets and reset prober metrics
	@./scripts/alerts/cleanup-all.sh