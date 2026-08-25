#!/usr/bin/env bash
# ==============================================================
# model-upload.sh
# Push downloaded models from ./model-cache/hub/ to Harbor as
# OCI artifacts using a containerised ORAS.
#
# Starts a kubectl port-forward automatically if the Harbor port
# is not already reachable (e.g. when called standalone). When
# invoked from model-sync.sh the existing port-forward is reused.
#
# ORAS push writes the full hub/models--<org>--<name>/ tree as
# a single OCI artifact. The ORAS pull Job in Kubernetes sets
# workingDir: /model-cache/hub so the pull recreates the same tree
# under /model-cache/hub/... matching HF_HUB_CACHE layout.
#
# Prerequisites:
#   - Docker (or compatible runtime)
#   - kubectl configured for the target cluster
#   - env-vars/<name> file with HARBOR_USER and HARBOR_PASSWORD set
#   - model-download.sh already run (./model-cache populated)
#
# Usage:
#   ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-upload.sh
#   KUBECONFIG=~/.kube/rke2-cluster.yaml ENV_FILE=... MODELS_YML=... bash model-upload.sh
# ==============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${SCRIPT_DIR}/logs"
LOG_FILE="${SCRIPT_DIR}/logs/model-upload-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "${LOG_FILE}") 2>&1

if [[ -z "${MODELS_YML:-}" ]]; then
  echo "ERROR: MODELS_YML is required. E.g.: MODELS_YML=<stack-name>.yml bash model-upload.sh" >&2
  exit 1
fi
[[ "${MODELS_YML}" != /* ]] && MODELS_YML="${SCRIPT_DIR}/models/${MODELS_YML}"
MODELS_YML="$(realpath "${MODELS_YML}")"
CACHE_DIR="${SCRIPT_DIR}/model-cache"
ORAS_IMAGE="${ORAS_IMAGE:-ghcr.io/oras-project/oras:v1.3.1}"
YQ_IMAGE="${YQ_IMAGE:-linuxserver/yq:3.4.3}"

HARBOR_HOST="${HARBOR_HOST:-localhost}"
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_REGISTRY="${HARBOR_HOST}:${HARBOR_LOCAL_PORT}"

# ── Load env ──────────────────────────────────────────────────────────────────
if [[ -z "${ENV_FILE:-}" ]]; then
  echo "ERROR: ENV_FILE is required. E.g.: ENV_FILE=<stack-name> bash model-upload.sh" >&2
  exit 1
fi
[[ "${ENV_FILE}" != /* ]] && ENV_FILE="${SCRIPT_DIR}/env-vars/${ENV_FILE}"
[[ -f "${ENV_FILE}" ]] && source "${ENV_FILE}"

# ── Logging ───────────────────────────────────────────────────────────────────
ts()      { date '+%H:%M:%S'; }
info()    { echo "$(ts) [INFO]  $*"; }
skip()    { echo "$(ts) [SKIP]  $*"; }
warn()    { echo "$(ts) [WARN]  $*" >&2; }
error()   { echo "$(ts) [ERROR] $*" >&2; }
section() { echo ""; echo "$(ts) ──── $* ────"; }

# ── Startup context ───────────────────────────────────────────────────────────
section "model-upload.sh"
info "Env file  : ${ENV_FILE}"
info "Models    : ${MODELS_YML}"
info "Registry  : ${HARBOR_REGISTRY}"

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ -z "${HARBOR_USER:-}" || -z "${HARBOR_PASSWORD:-}" ]]; then
  error "HARBOR_USER and HARBOR_PASSWORD are required. Set them in the env file or as env vars."
  exit 1
fi

if ! command -v docker &>/dev/null; then
  error "docker is required."
  exit 1
fi

# ── Port-forward (only if not already reachable) ──────────────────────────────
HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_SVC="${HARBOR_SVC:-harbor}"
PF_PID=""

KUBECTL_KUBECONFIG_ARG=()
if [[ -n "${KUBECONFIG:-}" ]]; then
  KUBECTL_KUBECONFIG_ARG=(--kubeconfig "${KUBECONFIG}")
  info "Kubeconfig: ${KUBECONFIG}"
fi

if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  info "Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → ${HARBOR_SVC}.${HARBOR_NAMESPACE}:80"
  kubectl "${KUBECTL_KUBECONFIG_ARG[@]}" port-forward \
    -n "${HARBOR_NAMESPACE}" \
    "svc/${HARBOR_SVC}" \
    "${HARBOR_LOCAL_PORT}:80" \
    > "${SCRIPT_DIR}/logs/harbor-portforward-upload.log" 2>&1 &
  PF_PID=$!
  for i in $(seq 1 10); do
    if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
    sleep 1
  done
  if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
    error "Port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}."
    cat "${SCRIPT_DIR}/logs/harbor-portforward-upload.log" >&2
    exit 1
  fi
  info "Port-forward ready."
else
  info "Port-forward already active on localhost:${HARBOR_LOCAL_PORT} — reusing."
fi

# ── Store ORAS credentials in a temp dir for the duration of the script ───────
CREDS_DIR="$(mktemp -d)"
trap 'rm -rf "${CREDS_DIR}"; [[ -n "${PF_PID}" ]] && kill "${PF_PID}" 2>/dev/null || true' EXIT

section "ORAS login"
info "Logging into Harbor (${HARBOR_REGISTRY})..."
echo "${HARBOR_PASSWORD}" | docker run --rm -i \
  --network host \
  -v "${CREDS_DIR}:/root/.docker" \
  "${ORAS_IMAGE}" \
  login --plain-http \
  --password-stdin \
  -u "${HARBOR_USER}" \
  "${HARBOR_REGISTRY}"

# linuxserver/yq prints an init banner to stdout before the actual yq output.
# Strip everything up to and including the "[ls.io-init] done." line.
yq() {
  docker run --rm -v "${MODELS_YML}:/models.yml:ro" "${YQ_IMAGE}" yq -r "$@" /models.yml \
    | awk '/\[ls\.io-init\] done\./{p=1; next} p'
}

model_count=$(yq '.models | length')
pushed=0
skipped=0

# ── Ensure refs/main exists for complete HF cache layout ──────────────────────
# huggingface-cli download --cache-dir sometimes omits the refs/ directory.
# Without refs/main the HF library cannot resolve the model offline
# (it maps branch → snapshot hash). We create it from the snapshot directory
# before pushing so the OCI artifact is self-contained.
ensure_refs_main() {
  local hub_path="$1"
  local snapshots_dir="${hub_path}/snapshots"
  local refs_dir="${hub_path}/refs"

  [[ -d "${snapshots_dir}" ]] || return 0

  # Iterate snapshot subdirectories. Use -d check to guard against the glob
  # not matching anything (bash keeps the literal pattern when nullglob is off).
  local snap
  for snap in "${snapshots_dir}"/*/; do
    [[ -d "${snap}" ]] || continue            # skip literal pattern if no match
    snap="${snap%/}"
    snap="${snap##*/}"
    [[ -n "${snap}" ]] || continue

    # model-download.sh already creates refs/main; this is a safety net for
    # caches built before that fix. download.sh pins revision so there is
    # always exactly one snapshot per freshly downloaded model.
    if [[ ! -f "${refs_dir}/main" ]]; then
      mkdir -p "${refs_dir}"
      printf "%s" "${snap}" > "${refs_dir}/main"
      info "  Created refs/main → ${snap} in ${hub_path##*/}"
    fi
    break
  done
}

# ── Push models ───────────────────────────────────────────────────────────────
section "Uploading ${model_count} model(s)"

for i in $(seq 0 $((model_count - 1))); do
  model_id=$(yq ".models[${i}].id")
  harbor_project=$(yq ".models[${i}].harbor_project")
  harbor_name=$(yq ".models[${i}].harbor_name")
  harbor_tag=$(yq ".models[${i}].harbor_tag")

  model_dir_name="models--$(echo "${model_id}" | sed 's|/|--|g')"
  model_hub_path="${CACHE_DIR}/hub/${model_dir_name}"

  if [[ ! -d "${model_hub_path}" ]]; then
    error "Model cache not found: ${model_hub_path} — run model-download.sh first."
    exit 1
  fi

  # Collect all directories to push: main model + dependencies
  push_dirs=("${model_dir_name}")

  dep_count=$(yq ".models[${i}].dependencies | length" 2>/dev/null || echo "0")
  for j in $(seq 0 $((dep_count - 1))); do
    dep_id=$(yq ".models[${i}].dependencies[${j}].id")
    dep_dir_name="models--$(echo "${dep_id}" | sed 's|/|--|g')"
    dep_hub_path="${CACHE_DIR}/hub/${dep_dir_name}"

    if [[ ! -d "${dep_hub_path}" ]]; then
      error "Dependency cache not found: ${dep_hub_path} — run model-download.sh first."
      exit 1
    fi

    push_dirs+=("${dep_dir_name}")
  done

  ref="${HARBOR_REGISTRY}/${harbor_project}/${harbor_name}:${harbor_tag}"

  # Ensure each model dir has refs/main before pushing.
  for dir in "${push_dirs[@]}"; do
    ensure_refs_main "${CACHE_DIR}/hub/${dir}"
  done

  echo ""
  if docker run --rm \
      --network host \
      -v "${CREDS_DIR}:/root/.docker" \
      "${ORAS_IMAGE}" \
      manifest fetch --plain-http "${ref}" > /dev/null 2>&1; then
    skip "${model_id} — ${ref} already exists in Harbor."
    ((skipped++)) || true
    continue
  fi

  info "Pushing ${model_id} → ${ref}"
  info "  Artifacts: ${push_dirs[*]}"

  # Total layers = one per push_dir + one OCI config blob + one manifest.
  total_layers=$(( ${#push_dirs[@]} + 2 ))
  total_size=0
  for dir in "${push_dirs[@]}"; do
    sz=$(find "${CACHE_DIR}/hub/${dir}" -type f -printf '%s\n' 2>/dev/null | awk '{s+=$1} END{print s+0}')
    total_size=$(( total_size + sz ))
  done
  info "  Total : $(numfmt --to=iec "${total_size}" 2>/dev/null || echo "${total_size}B") in ${total_layers} layer(s)"

  # Pipe push output through a line filter — print one progress line per
  # completed layer ("Uploaded" or "Exists" from ORAS).
  _done=0
  docker run --rm \
    --network host \
    --workdir /model-cache/hub \
    -v "${CREDS_DIR}:/root/.docker" \
    -v "${CACHE_DIR}/hub:/model-cache/hub:ro" \
    "${ORAS_IMAGE}" \
    push --plain-http \
    --artifact-type "application/vnd.cnai.model" \
    "${ref}" \
    "${push_dirs[@]}" 2>&1 | while IFS= read -r line; do
    printf '%s\n' "${line}"
    if [[ "${line}" == Uploaded* || "${line}" == Exists* ]]; then
      (( _done++ )) || true
      info "  progress: ${_done} / ${total_layers} layers"
    fi
  done

  info "Pushed ${ref}"
  ((pushed++)) || true
done

# ── Summary ───────────────────────────────────────────────────────────────────
section "Upload summary"
info "Pushed  : ${pushed}"
info "Skipped : ${skipped} (already in Harbor)"
