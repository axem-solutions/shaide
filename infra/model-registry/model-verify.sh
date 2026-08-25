#!/usr/bin/env bash
# ==============================================================
# model-verify.sh
# Verify that models declared in models/<name>.yml are present
# in Harbor as OCI artifacts. For each model:
#   1. Check manifest exists (oras manifest fetch)
#   2. Pull the artifact to a temp dir
#   3. Verify that all expected hub directories are present
#
# Starts a kubectl port-forward automatically if Harbor is not
# already reachable (same pattern as model-upload.sh).
#
# Prerequisites:
#   - Docker (or compatible runtime)
#   - kubectl configured for the target cluster
#   - env-vars/<name> file with HARBOR_USER and HARBOR_PASSWORD set
#
# Usage:
#   ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-verify.sh
# ==============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${SCRIPT_DIR}/logs"
LOG_FILE="${SCRIPT_DIR}/logs/model-verify-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "${LOG_FILE}") 2>&1

if [[ -z "${MODELS_YML:-}" ]]; then
  echo "ERROR: MODELS_YML is required. E.g.: MODELS_YML=<stack-name>.yml bash model-verify.sh" >&2
  exit 1
fi
[[ "${MODELS_YML}" != /* ]] && MODELS_YML="${SCRIPT_DIR}/models/${MODELS_YML}"
MODELS_YML="$(realpath "${MODELS_YML}")"

ORAS_IMAGE="${ORAS_IMAGE:-ghcr.io/oras-project/oras:v1.3.1}"
YQ_IMAGE="${YQ_IMAGE:-linuxserver/yq:3.4.3}"

HARBOR_HOST="${HARBOR_HOST:-localhost}"
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_REGISTRY="${HARBOR_HOST}:${HARBOR_LOCAL_PORT}"

# ── Load env ──────────────────────────────────────────────────────────────────
if [[ -z "${ENV_FILE:-}" ]]; then
  echo "ERROR: ENV_FILE is required. E.g.: ENV_FILE=<stack-name> bash model-verify.sh" >&2
  exit 1
fi
[[ "${ENV_FILE}" != /* ]] && ENV_FILE="${SCRIPT_DIR}/env-vars/${ENV_FILE}"
[[ -f "${ENV_FILE}" ]] && source "${ENV_FILE}"

# ── Logging ───────────────────────────────────────────────────────────────────
ts()      { date '+%H:%M:%S'; }
info()    { echo "$(ts) [INFO]  $*"; }
pass()    { echo "$(ts) [PASS]  $*"; }
fail()    { echo "$(ts) [FAIL]  $*"; }
skip()    { echo "$(ts) [SKIP]  $*"; }
warn()    { echo "$(ts) [WARN]  $*" >&2; }
error()   { echo "$(ts) [ERROR] $*" >&2; }
section() { echo ""; echo "$(ts) ──── $* ────"; }

# ── Startup context ───────────────────────────────────────────────────────────
section "model-verify.sh"
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

if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  info "Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → ${HARBOR_SVC}.${HARBOR_NAMESPACE}:80"
  kubectl port-forward \
    -n "${HARBOR_NAMESPACE}" \
    "svc/${HARBOR_SVC}" \
    "${HARBOR_LOCAL_PORT}:80" \
    > "${SCRIPT_DIR}/logs/harbor-portforward-verify.log" 2>&1 &
  PF_PID=$!
  for i in $(seq 1 10); do
    if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
    sleep 1
  done
  if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
    error "Port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}."
    cat "${SCRIPT_DIR}/logs/harbor-portforward-verify.log" >&2
    exit 1
  fi
  info "Port-forward ready."
else
  info "Port-forward already active on localhost:${HARBOR_LOCAL_PORT} — reusing."
fi

# ── Credentials and temp dirs ─────────────────────────────────────────────────
CREDS_DIR="$(mktemp -d)"
PULL_DIR="$(mktemp -d)"
trap 'rm -rf "${CREDS_DIR}" "${PULL_DIR}"; [[ -n "${PF_PID}" ]] && kill "${PF_PID}" 2>/dev/null || true' EXIT

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
passed=0
failed=0

# ── Verify models ─────────────────────────────────────────────────────────────
section "Verifying ${model_count} model(s)"

for i in $(seq 0 $((model_count - 1))); do
  model_id=$(yq ".models[${i}].id")
  harbor_project=$(yq ".models[${i}].harbor_project")
  harbor_name=$(yq ".models[${i}].harbor_name")
  harbor_tag=$(yq ".models[${i}].harbor_tag")

  ref="${HARBOR_REGISTRY}/${harbor_project}/${harbor_name}:${harbor_tag}"

  echo ""
  info "Verifying ${model_id}"
  info "  Ref: ${ref}"

  # ── Step 1: Check manifest ─────────────────────────────────────────────────
  info "  [1/3] Checking manifest..."
  if ! docker run --rm \
      --network host \
      -v "${CREDS_DIR}:/root/.docker" \
      "${ORAS_IMAGE}" \
      manifest fetch --plain-http "${ref}" > /dev/null 2>&1; then
    fail "${model_id} — manifest not found in Harbor: ${ref}"
    ((failed++)) || true
    continue
  fi
  info "        Manifest: OK"

  # ── Step 2: Pull artifact ──────────────────────────────────────────────────
  info "  [2/3] Pulling artifact..."
  MODEL_PULL_DIR="${PULL_DIR}/${harbor_name}"
  mkdir -p "${MODEL_PULL_DIR}"

  if ! docker run --rm \
      --user "$(id -u):$(id -g)" \
      --network host \
      --workdir /pull \
      -v "${CREDS_DIR}:/root/.docker" \
      -v "${MODEL_PULL_DIR}:/pull" \
      "${ORAS_IMAGE}" \
      pull --plain-http "${ref}" > /dev/null 2>&1; then
    fail "${model_id} — pull failed"
    ((failed++)) || true
    continue
  fi
  info "        Pull: OK"

  # ── Step 3: Verify expected directories ───────────────────────────────────
  info "  [3/3] Checking artifact contents..."

  check_failed=0

  # Checks one model directory for the required HF cache layout:
  #   models--org--name/blobs/       — actual file content
  #   models--org--name/snapshots/   — at least one snapshot subdirectory
  #   models--org--name/refs/main    — maps "main" branch → snapshot hash
  # Without refs/main the HF library cannot resolve the model offline.
  check_model_dir() {
    local pull_dir="$1"
    local dir_name="$2"
    local label="$3"
    local dir="${pull_dir}/${dir_name}"

    if [[ ! -d "${dir}" ]]; then
      fail "${label} — directory missing after pull: ${dir_name}"
      return 1
    fi
    info "        ${dir_name}: found"

    local ok=0

    if [[ ! -d "${dir}/blobs" ]]; then
      fail "${label} — blobs/ missing in ${dir_name}"
      ok=1
    else
      info "        ${dir_name}/blobs: found"
    fi

    local snap_count
    snap_count=$(find "${dir}/snapshots" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
    if [[ "${snap_count}" -eq 0 ]]; then
      fail "${label} — no snapshot subdirectory in ${dir_name}/snapshots"
      ok=1
    else
      info "        ${dir_name}/snapshots: found (${snap_count} snapshot(s))"
    fi

    if [[ ! -f "${dir}/refs/main" ]]; then
      fail "${label} — refs/main missing in ${dir_name}"
      ok=1
    else
      local ref_hash
      ref_hash=$(cat "${dir}/refs/main")
      if [[ -z "${ref_hash}" ]]; then
        fail "${label} — refs/main is empty in ${dir_name}"
        ok=1
      else
        info "        ${dir_name}/refs/main: ${ref_hash}"
      fi
    fi

    return ${ok}
  }

  # Main model directory
  model_dir_name="models--$(echo "${model_id}" | sed 's|/|--|g')"
  if ! check_model_dir "${MODEL_PULL_DIR}" "${model_dir_name}" "${model_id}"; then
    check_failed=1
  fi

  # Dependency directories
  dep_count=$(yq ".models[${i}].dependencies | length" 2>/dev/null || echo "0")
  for j in $(seq 0 $((dep_count - 1))); do
    dep_id=$(yq ".models[${i}].dependencies[${j}].id")
    dep_dir_name="models--$(echo "${dep_id}" | sed 's|/|--|g')"
    if ! check_model_dir "${MODEL_PULL_DIR}" "${dep_dir_name}" "${model_id} dep:${dep_id}"; then
      check_failed=1
    fi
  done

  if [[ ${check_failed} -eq 0 ]]; then
    pass "${model_id} — all checks passed"
    ((passed++)) || true
  else
    ((failed++)) || true
  fi

  rm -rf "${MODEL_PULL_DIR}"
done

# ── Summary ───────────────────────────────────────────────────────────────────
section "Verify summary"
info "Passed  : ${passed}"
info "Failed  : ${failed}"

[[ ${failed} -eq 0 ]]
