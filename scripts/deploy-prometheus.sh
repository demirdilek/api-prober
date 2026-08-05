#!/usr/bin/env bash
set -euo pipefail

# Helm-Repository hinzufügen und aktualisieren
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

HELM_FLAGS=(
  "--set" "alertmanager.config.receivers[1].slack_configs[0].api_url=${SLACK_WEBHOOK_URL:-}"
  "--set" "alertmanager.config.receivers[2].pushover_configs[0].user_key=${PUSHOVER_USER_KEY:-}"
  "--set" "alertmanager.config.receivers[2].pushover_configs[0].token=${PUSHOVER_API_TOKEN:-}"
)

if [ -f prom-stack-values.local.yaml ]; then
  echo "==> Applying Prometheus stack with local overrides and .env secrets..."
  helm upgrade --install prom-stack prometheus-community/kube-prometheus-stack \
    -f prom-stack-values.yaml \
    -f prom-stack-values.local.yaml \
    "${HELM_FLAGS[@]}"
else
  echo "==> Applying base Prometheus stack configuration with .env secrets..."
  helm upgrade --install prom-stack prometheus-community/kube-prometheus-stack \
    -f prom-stack-values.yaml \
    "${HELM_FLAGS[@]}"
fi