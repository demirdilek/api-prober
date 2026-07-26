.PHONY: help docker-build helm-install helm-upgrade helm-uninstall deploy
.DEFAULT_GOAL := help

# Container registry configuration for Retail Edge
IMAGE_REPO=ghcr.io/demirdilek/api-prober
IMAGE_TAG=latest

# Helm variables
RELEASE_NAME=api-prober
CHART_DIR=./helm/api-prober

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

docker-build: ## Build the docker image locally (only needed for local testing)
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

helm-install: ## Install the application into the cluster
	helm install $(RELEASE_NAME) $(CHART_DIR)

helm-upgrade: ## Upgrade the existing release (pulls the new image from ghcr.io)
	helm upgrade $(RELEASE_NAME) $(CHART_DIR)

helm-uninstall: ## Completely remove the application from the cluster
	helm uninstall $(RELEASE_NAME)

deploy: helm-upgrade ## Shortcut to deploy updates from the registry