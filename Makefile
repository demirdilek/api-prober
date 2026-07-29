-include .env
export

.PHONY: help k3d-up vault-up docker-build eso-install vault-secrets prometheus-install helm-install all helm-upgrade helm-uninstall local-deploy hard-reset vault-down k3d-down clean forward-all stop-forward test iinstall-argocd apply-gitops argocd-pass

.DEFAULT_GOAL := help

# Container registry configuration
IMAGE_REPO=ghcr.io/demirdilek/api-prober
IMAGE_TAG=dev

ARGOCD_MANIFEST_URL ?= https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

TAILSCALE_IP ?= $(shell tailscale ip -4 2>/dev/null || echo "localhost")

# Helm variables
RELEASE_NAME=api-prober
CHART_DIR=./helm/api-prober

# Vault Docker settings for local dev
VAULT_CONTAINER_NAME=vault-dev-server
VAULT_DEV_TOKEN=myroottoken

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

k3d-up: ## 1. Create a local k3d Kubernetes cluster
	@if k3d cluster list | grep -q "mycluster"; then \
		echo "Cluster 'mycluster' already exists."; \
	else \
		k3d cluster create mycluster --api-port 6443 -p "80:80@loadbalancer" -p "443:443@loadbalancer" --agents 2; \
	fi

vault-up: ## 2. Start local HashiCorp Vault container & seed secrets
	@if [ ! -f .env ]; then \
		echo "Error: .env file missing! Copy .env.example to .env and set your credentials."; \
		exit 1; \
	fi
	@if docker ps | grep -q "$(VAULT_CONTAINER_NAME)"; then \
		echo "Vault container already running."; \
	else \
		echo "Starting local HashiCorp Vault..."; \
		docker run -d --name $(VAULT_CONTAINER_NAME) \
			-p 8200:8200 \
			-e VAULT_DEV_ROOT_TOKEN_ID=$(VAULT_DEV_TOKEN) \
			hashicorp/vault:latest; \
		echo "Waiting for Vault to initialize..."; \
		sleep 4; \
		docker exec -e VAULT_ADDR='http://127.0.0.1:8200' $(VAULT_CONTAINER_NAME) \
			vault kv put secret/pushover user_key="$(PUSHOVER_USER_KEY)" api_token="$(PUSHOVER_API_TOKEN)"; \
		echo "Vault initialized and secrets seeded at secret/pushover!"; \
	fi

docker-build: ## 3. Build local Docker image and import into k3d
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster

eso-install: ## 4. Install External Secrets Operator
	helm repo add external-secrets https://charts.external-secrets.io
	helm repo update
	@if ! helm status external-secrets > /dev/null 2>&1; then \
		echo "Installing External Secrets Operator..."; \
		helm install external-secrets external-secrets/external-secrets \
			--set installCRDs=true \
			--set webhook.create=false \
			--set certController.create=false \
			--wait; \
		echo "Waiting for CRD registration in Kubernetes API server..."; \
		until kubectl api-resources | grep -q "secretstores"; do sleep 1; done; \
	else \
		echo "External Secrets Operator already installed."; \
	fi

vault-secrets: ## 5. Connect Vault to k3d & sync Kubernetes secrets via ESO
	@sleep 3
	kubectl apply -f vault-external.yaml || true
	kubectl create secret generic vault-token --from-literal=token=$(VAULT_DEV_TOKEN) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f vault-store.yaml
	kubectl apply -f externalsecret.yaml

prometheus-install: ## 6. Install kube-prometheus-stack via Helm
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update
	@if ! helm status prom-stack > /dev/null 2>&1; then \
		helm install prom-stack prometheus-community/kube-prometheus-stack -f prom-stack-values.yaml; \
	else \
		echo "Prometheus stack already installed."; \
	fi

helm-install: ## 7. Deploy application Helm chart (api-prober)
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR)

local-deploy: ## 8. Build fresh without cache, import to k3d, and restart deployment
	docker build --no-cache -t $(IMAGE_REPO):$(IMAGE_TAG) .
	k3d image import $(IMAGE_REPO):$(IMAGE_TAG) -c mycluster
	kubectl rollout restart deployment $(RELEASE_NAME)

hard-reset: ## 9. Deep clean docker system, delete cluster, and rebuild everything fresh
	docker system prune -a --volumes -f
	$(MAKE) clean
	$(MAKE) all

all: k3d-up vault-up docker-build eso-install vault-secrets prometheus-install install-argocd apply-gitops helm-install
	@echo "========================================================="
	@echo " api-prober stack is fully up and running out-of-the-box! "
	@echo "========================================================="

helm-upgrade: ## Upgrade existing api-prober Helm release
	helm upgrade $(RELEASE_NAME) $(CHART_DIR)

helm-uninstall: ## Remove api-prober Helm release
	helm uninstall $(RELEASE_NAME) || true

vault-down: ## Stop and remove Vault container
	@docker stop $(VAULT_CONTAINER_NAME) || true
	@docker rm $(VAULT_CONTAINER_NAME) || true

k3d-down: vault-down ## Delete local k3d cluster & stop Vault
	k3d cluster delete mycluster || true

clean: k3d-down ## Clean up cluster, containers and temporary files
	rm -f project-dump.txt

forward-all: ## Forward Argo CD, Prometheus & Grafana UIs
	@echo "========================================================"
	@echo " CONTROL PLANE WEB UIs (Tailscale / iPhone Access)"
	@echo "========================================================"
	@echo " ARGO CD:    https://$(TAILSCALE_IP):8080"
	@echo "   User:     admin"
	@echo -n "   Password: "
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d 2>/dev/null || echo "Not found"
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

test: ## Run unit and integration tests
	go test -v -race ./...

install-argocd: ## install-argocd: Install Argo CD components into the cluster (Server-Side Apply)
	@echo "==> Installing Argo CD..."
	kubectl create namespace argocd || true
	kubectl apply -n argocd --server-side --force-conflicts -f $(ARGOCD_MANIFEST_URL)
	@echo "==> Waiting for Argo CD components to be ready..."
	kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=300s

apply-gitops: ## apply-gitops: Register the api-prober application in Argo CD
	@echo "==> Registering api-prober Application in Argo CD..."
	kubectl apply -f deploy/argocd/api-prober-app.yaml

argocd-pass: ## argocd-pass: Retrieve the initial admin password for Argo CD UI
	@echo "==> Argo CD Initial Admin Password:"
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d; echo ""
