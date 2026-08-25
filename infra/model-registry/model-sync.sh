#!/usr/bin/env bash
# ==============================================================
# model-sync.sh
# Convenience wrapper: downloads models from HuggingFace, opens
# a kubectl port-forward to Harbor, then pushes all models as
# OCI artifacts using model-upload.sh.
#
# Works for both GCP and on-prem Harbor. Harbor is ClusterIP-only
# so the port-forward is opened here and torn down on exit.
#
# Prerequisites:
#   - Docker (or compatible runtime)
#   - kubectl configured for the target cluster
#   - env-vars/<name> file with HF_TOKEN, HARBOR_USER, HARBOR_PASSWORD set
#
# Usage:
#   ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-sync.sh
#
# On-prem:
#   KUBECONFIG=~/.kube/rke2-cluster.yaml ENV_FILE=... MODELS_YML=... bash model-sync.sh
# ==============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${SCRIPT_DIR}/logs"
LOG_FILE="${SCRIPT_DIR}/logs/model-sync-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "${LOG_FILE}") 2>&1

# ── Load env ──────────────────────────────────────────────────────────────────
if [[ -z "${ENV_FILE:-}" ]]; then
  echo "ERROR: ENV_FILE is required. E.g.: ENV_FILE=<stack-name> bash model-sync.sh" >&2
  exit 1
fi
if [[ -z "${MODELS_YML:-}" ]]; then
  echo "ERROR: MODELS_YML is required. E.g.: MODELS_YML=<stack-name>.yml bash model-sync.sh" >&2
  exit 1
fi
[[ "${ENV_FILE}" != /* ]] && ENV_FILE="${SCRIPT_DIR}/env-vars/${ENV_FILE}"
[[ -f "${ENV_FILE}" ]] && source "${ENV_FILE}"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_SVC="${HARBOR_SVC:-harbor}"
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"

# ── Logging ───────────────────────────────────────────────────────────────────
ts()      { date '+%H:%M:%S'; }
info()    { echo "$(ts) [INFO]  $*"; }
warn()    { echo "$(ts) [WARN]  $*" >&2; }
error()   { echo "$(ts) [ERROR] $*" >&2; }
section() { echo ""; echo "$(ts) ──── $* ────"; }

# ── Startup context ───────────────────────────────────────────────────────────
section "model-sync.sh"
info "Env file  : ${ENV_FILE}"
info "Models    : ${SCRIPT_DIR}/models/${MODELS_YML}"
info "Registry  : localhost:${HARBOR_LOCAL_PORT}"

# ── Validate ──────────────────────────────────────────────────────────────────
for var in HF_TOKEN HARBOR_USER HARBOR_PASSWORD; do
  if [[ -z "${!var:-}" ]]; then
    error "${var} is required. Set it in the env file or as an env var."
    exit 1
  fi
done

if ! command -v docker &>/dev/null; then
  error "docker is required."
  exit 1
fi

# ── Build kubectl args for optional kubeconfig ────────────────────────────────
kubectl_args=()
if [[ -n "${KUBECONFIG:-}" ]]; then
  kubectl_args=(--kubeconfig "${KUBECONFIG}")
  info "Kubeconfig: ${KUBECONFIG}"
fi

# ── Download ──────────────────────────────────────────────────────────────────
section "Download phase"
bash "${SCRIPT_DIR}/model-download.sh"

# ── Start port-forward (upload only) ─────────────────────────────────────────
section "Port-forward"
info "Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → ${HARBOR_SVC}.${HARBOR_NAMESPACE}:80"
kubectl "${kubectl_args[@]}" port-forward \
  -n "${HARBOR_NAMESPACE}" \
  "svc/${HARBOR_SVC}" \
  "${HARBOR_LOCAL_PORT}:80" \
  > "${SCRIPT_DIR}/logs/harbor-portforward-sync.log" 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true; echo ""; info "Port-forward closed."' EXIT

for i in $(seq 1 10); do
  if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
  sleep 1
done
if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  error "Port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}."
  error "Check ${SCRIPT_DIR}/logs/harbor-portforward-sync.log for details."
  exit 1
fi
info "Port-forward ready."

# ── Upload ────────────────────────────────────────────────────────────────────
section "Upload phase"
HARBOR_HOST="localhost" HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT}" \
  bash "${SCRIPT_DIR}/model-upload.sh"

section "Sync complete"
info "Done."
