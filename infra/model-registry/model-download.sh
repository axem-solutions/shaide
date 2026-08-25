#!/usr/bin/env bash
# ==============================================================
# model-download.sh
# Download HuggingFace models declared in models/<name>.yml into
# ./model-cache/hub/ using a containerised huggingface-cli.
#
# The HF token stays on the provisioner laptop — it is never
# passed to any cluster or stored in Kubernetes secrets.
#
# Prerequisites:
#   - Docker (or compatible runtime)
#   - env-vars/<name> file with HF_TOKEN set (copy from env-vars/example)
#
# Usage:
#   ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-download.sh
# ==============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${SCRIPT_DIR}/logs"
LOG_FILE="${SCRIPT_DIR}/logs/model-download-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "${LOG_FILE}") 2>&1

if [[ -z "${MODELS_YML:-}" ]]; then
  echo "ERROR: MODELS_YML is required. E.g.: MODELS_YML=<stack-name>.yml bash model-download.sh" >&2
  exit 1
fi
[[ "${MODELS_YML}" != /* ]] && MODELS_YML="${SCRIPT_DIR}/models/${MODELS_YML}"
MODELS_YML="$(realpath "${MODELS_YML}")"
CACHE_DIR="${SCRIPT_DIR}/model-cache"
HF_IMAGE="hf-downloader:latest"
YQ_IMAGE="${YQ_IMAGE:-linuxserver/yq:3.4.3}"

# ── Load env ──────────────────────────────────────────────────────────────────
if [[ -z "${ENV_FILE:-}" ]]; then
  echo "ERROR: ENV_FILE is required. E.g.: ENV_FILE=<stack-name> bash model-download.sh" >&2
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
section "model-download.sh"
info "Env file  : ${ENV_FILE}"
info "Models    : ${MODELS_YML}"
info "Cache dir : ${CACHE_DIR}/hub"

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ -z "${HF_TOKEN:-}" ]]; then
  error "HF_TOKEN is required. Set it in the env file or as an env var."
  exit 1
fi

if ! command -v docker &>/dev/null; then
  error "docker is required."
  exit 1
fi

# ── Build downloader image if needed ─────────────────────────────────────────
if ! docker image inspect "${HF_IMAGE}" &>/dev/null; then
  info "Building downloader image..."
  docker build -t "${HF_IMAGE}" "${SCRIPT_DIR}/images/downloader"
fi

mkdir -p "${CACHE_DIR}"

# linuxserver/yq prints an init banner to stdout before the actual yq output.
# Strip everything up to and including the "[ls.io-init] done." line.
yq() {
  docker run --rm -v "${MODELS_YML}:/models.yml:ro" "${YQ_IMAGE}" yq -r "$@" /models.yml \
    | awk '/\[ls\.io-init\] done\./{p=1; next} p'
}

# ── download_model <model_id> [revision] ──────────────────────────────────────
# Downloads a single model into the HF cache and creates refs/main.
# Uses a sentinel file (.complete) to distinguish a finished download from an
# interrupted one — avoids silently serving a partial cache on re-runs.
download_model() {
  local model_id="$1"
  local revision="${2:-}"
  local model_dir="${CACHE_DIR}/hub/models--$(echo "${model_id}" | sed 's|/|--|g')"
  local sentinel="${model_dir}/.complete"

  echo ""
  if [[ -f "${sentinel}" ]]; then
    skip "${model_id} — already in cache (sentinel present)."
    return 0
  fi

  if [[ -d "${model_dir}" ]]; then
    warn "${model_id} — directory exists but no sentinel; previous download may be incomplete. Re-downloading."
    rm -rf "${model_dir}"
    # Also clean xet temp data — stale CAS chunks from the interrupted download
    # can corrupt the progress baseline and occasionally cause xet to misbehave.
    rm -rf "${CACHE_DIR}/xet"
    info "  Cleaned up incomplete model dir and xet cache."
  fi

  info "Downloading ${model_id} ..."

  local revision_args=()
  if [[ -n "${revision}" && "${revision}" != "null" && "${revision}" != '""' ]]; then
    revision_args=(--revision "${revision}")
    info "  Pinned revision: ${revision}"
  fi

  # Query total model size from the HF API before download so the monitor can
  # show "downloaded / total (pct%)". Uses the hf-downloader image (already
  # built). Variables are passed via env to avoid bash-expansion in Python.
  local total_bytes=0
  local total_files=0
  local size_info
  local _rev="${revision}"
  [[ "${_rev}" == "null" ]] && _rev="main"
  size_info=$(docker run --rm \
    --entrypoint python3 \
    -e HF_TOKEN="${HF_TOKEN}" \
    -e HF_MODEL="${model_id}" \
    -e HF_REV="${_rev:-main}" \
    "${HF_IMAGE}" \
    -c '
import json, os, sys, urllib.request
token = os.environ.get("HF_TOKEN", "")
model = os.environ["HF_MODEL"]
rev   = os.environ.get("HF_REV", "main") or "main"
url = "https://huggingface.co/api/models/" + model + "?blobs=true&revision=" + rev
req = urllib.request.Request(url, headers={"Authorization": "Bearer " + token} if token else {})
try:
    with urllib.request.urlopen(req, timeout=15) as r:
        data = json.loads(r.read())
    siblings = data.get("siblings", [])
    total = sum(s.get("size", 0) for s in siblings if s.get("size"))
    print(total, len(siblings))
except Exception as e:
    print("ERROR: " + str(e), file=sys.stderr)
    sys.exit(1)
' 2>&1) || true
  if [[ "${size_info}" =~ ^[0-9]+[[:space:]][0-9]+$ ]]; then
    total_bytes=$(echo "${size_info}" | awk '{print $1}')
    total_files=$(echo "${size_info}" | awk '{print $2}')
    info "  Total size : $(numfmt --to=iec "${total_bytes}" 2>/dev/null || echo "${total_bytes}B") in ${total_files} file(s)"
  else
    warn "  Could not fetch total size: ${size_info}"
  fi

  # Pipe docker output through a line filter that prints a progress line
  # immediately after each "Download complete." — guarantees correct ordering
  # without a background process or polling.
  #
  # File count uses a simple counter (not find|wc-l) to avoid a race where
  # "Download complete." is logged before the rename() syscall returns, causing
  # the filesystem query to still see the .incomplete file.
  local _done_file="/tmp/_dl_done_$$"
  local _done=0
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e HF_TOKEN="${HF_TOKEN}" \
    -e HF_HOME=/model-cache \
    -e HF_HUB_ENABLE_HF_TRANSFER=1 \
    -e HF_XET_CACHE=/model-cache/xet \
    -v "${CACHE_DIR}:/model-cache" \
    "${HF_IMAGE}" \
    download "${revision_args[@]}" \
      --cache-dir /model-cache/hub \
      "${model_id}" 2>&1 | while IFS= read -r line; do
    printf '%s\n' "${line}"
    if [[ "${line}" == "Download complete."* ]]; then
      (( _done++ )) || true
      bytes_label=""
      if [[ "${total_bytes}" -gt 0 ]]; then
        complete_bytes=$(find "${model_dir}/blobs" -type f ! -name "*.incomplete" -printf '%s\n' 2>/dev/null | awk '{s+=$1} END{print s+0}')
        complete_bytes=${complete_bytes:-0}
        complete_hr=$(numfmt --to=iec "${complete_bytes}" 2>/dev/null || echo "${complete_bytes}B")
        total_hr=$(numfmt --to=iec "${total_bytes}" 2>/dev/null || echo "${total_bytes}B")
        pct=$(( complete_bytes * 100 / total_bytes ))
        bytes_label=" — ${complete_hr} / ${total_hr} (${pct}%)"
      fi
      info "  progress: ${_done} / ${total_files} files${bytes_label}"
      echo "${_done}" > "${_done_file}"
    fi
  done

  # If some blobs were reused (same content hash within or across models),
  # HF skips them silently — no "Download complete." line, counter falls short.
  # Print one final line with the correct logical file count only when needed.
  #
  # Counting blobs (blobs/ directory) undercounts when two logical files share
  # the same blob hash. The snapshot directory has one symlink per logical file
  # regardless of blob sharing, so symlink count == logical file count.
  # Following symlinks with -follow lets find report each target's size once per
  # symlink — shared blobs are counted twice, matching the HF API's total_bytes.
  local _last_done=0
  [[ -f "${_done_file}" ]] && _last_done=$(cat "${_done_file}") && rm -f "${_done_file}"
  if [[ "${_last_done}" -lt "${total_files}" ]]; then
    local final_count final_bytes final_hr total_hr pct snap_dir
    snap_dir=""
    for _d in "${model_dir}/snapshots"/*/; do
      [[ -d "${_d}" ]] && snap_dir="${_d}" && break
    done

    if [[ -n "${snap_dir}" ]]; then
      final_count=$(find "${snap_dir}" -type l 2>/dev/null | wc -l | tr -d ' ')
      final_count=${final_count:-0}
      if [[ "${total_bytes}" -gt 0 ]]; then
        final_bytes=$(find "${snap_dir}" -follow -type f -printf '%s\n' 2>/dev/null | awk '{s+=$1} END{print s+0}')
        final_bytes=${final_bytes:-0}
        final_hr=$(numfmt --to=iec "${final_bytes}" 2>/dev/null || echo "${final_bytes}B")
        total_hr=$(numfmt --to=iec "${total_bytes}" 2>/dev/null || echo "${total_bytes}B")
        pct=$(( final_bytes * 100 / total_bytes ))
        # HF API sizes are approximate; clamp to 100 when all files are present.
        [[ "${final_count}" -ge "${total_files}" ]] && pct=100
        info "  progress: ${final_count} / ${total_files} files — ${final_hr} / ${total_hr} (${pct}%)"
      else
        info "  progress: ${final_count} / ${total_files} files"
      fi
    else
      # Snapshot dir not yet present (very early failure) — fall back to blob count.
      local blob_count blob_bytes
      blob_count=$(find "${model_dir}/blobs" -type f ! -name "*.incomplete" 2>/dev/null | wc -l | tr -d ' ')
      blob_count=${blob_count:-0}
      if [[ "${total_bytes}" -gt 0 ]]; then
        blob_bytes=$(find "${model_dir}/blobs" -type f ! -name "*.incomplete" -printf '%s\n' 2>/dev/null | awk '{s+=$1} END{print s+0}')
        blob_bytes=${blob_bytes:-0}
        final_hr=$(numfmt --to=iec "${blob_bytes}" 2>/dev/null || echo "${blob_bytes}B")
        total_hr=$(numfmt --to=iec "${total_bytes}" 2>/dev/null || echo "${total_bytes}B")
        pct=$(( blob_bytes * 100 / total_bytes ))
        info "  progress: ${blob_count} / ${total_files} files — ${final_hr} / ${total_hr} (${pct}%)"
      else
        info "  progress: ${blob_count} / ${total_files} files"
      fi
    fi
  fi

  # huggingface-cli download --cache-dir creates blobs/ and snapshots/ but does
  # not write refs/. Without refs/main the HF library cannot resolve the model
  # offline (it maps branch name → snapshot hash). Create it now so the cache
  # is complete before it is pushed to Harbor.
  local snapshots_dir="${model_dir}/snapshots"
  if [[ -d "${snapshots_dir}" ]]; then
    local snap
    for snap in "${snapshots_dir}"/*/; do
      [[ -d "${snap}" ]] || continue          # skip if glob didn't match
      snap="${snap%/}"
      snap="${snap##*/}"
      [[ -n "${snap}" ]] || continue
      mkdir -p "${model_dir}/refs"
      printf "%s" "${snap}" > "${model_dir}/refs/main"
      info "  Created refs/main → ${snap}"
      break  # one snapshot per freshly-downloaded model
    done
  fi

  touch "${sentinel}"
  info "Downloaded ${model_id}."
}

model_count=$(yq '.models | length')
downloaded=0
skipped=0

# ── Download models ───────────────────────────────────────────────────────────
section "Downloading ${model_count} model(s)"

for i in $(seq 0 $((model_count - 1))); do
  model_id=$(yq ".models[${i}].id")
  revision=$(yq ".models[${i}].revision")

  model_dir="${CACHE_DIR}/hub/models--$(echo "${model_id}" | sed 's|/|--|g')"
  sentinel="${model_dir}/.complete"

  if [[ -f "${sentinel}" ]]; then
    skip "${model_id} — already in cache."
    ((skipped++)) || true
  else
    download_model "${model_id}" "${revision}"
    ((downloaded++)) || true
  fi

  dep_count=$(yq ".models[${i}].dependencies | length" 2>/dev/null || echo "0")
  for j in $(seq 0 $((dep_count - 1))); do
    dep_id=$(yq ".models[${i}].dependencies[${j}].id")
    dep_revision=$(yq ".models[${i}].dependencies[${j}].revision" 2>/dev/null || true)
    dep_dir="${CACHE_DIR}/hub/models--$(echo "${dep_id}" | sed 's|/|--|g')"
    dep_sentinel="${dep_dir}/.complete"

    if [[ -f "${dep_sentinel}" ]]; then
      skip "  dependency ${dep_id} — already in cache."
      ((skipped++)) || true
    else
      download_model "${dep_id}" "${dep_revision}"
      ((downloaded++)) || true
    fi
  done
done

# ── Summary ───────────────────────────────────────────────────────────────────
section "Download summary"
info "Downloaded : ${downloaded}"
info "Skipped    : ${skipped} (already cached)"
info "Cache dir  : ${CACHE_DIR}/hub"
