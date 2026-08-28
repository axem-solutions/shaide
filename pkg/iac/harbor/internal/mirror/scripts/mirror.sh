set -euo pipefail

# Single implementation for all three mirror paths (public images, pinned
# GHCR images, discovered private GHCR images) — they differ only in whether
# the source needs authentication and where the image list comes from, which
# used to mean three near-identical copies of the same
# digest-check/copy/failure-tracking logic.
#
# Controlled entirely by env vars:
#   MIRROR_REQUIRES_AUTH  "true" to log in to ghcr.io first and use
#                         authenticated source access; unset/"false" for
#                         anonymous source access (public images).
#   GHCR_USER, GHCR_TOKEN required when MIRROR_REQUIRES_AUTH=true; the run
#                         skips entirely (not "attempt anonymously") if
#                         either is missing, matching the never-guess-at
#                         private-image auth stance the callers rely on.
#   MIRROR_LIST           inline "src|dest" list, one per line.
#   MIRROR_LIST_FILE      "src|dest" list read from a file instead — set
#                         at most one of MIRROR_LIST / MIRROR_LIST_FILE.
#   HARBOR_HOST, HARBOR_ROBOT_USER, HARBOR_ROBOT_PASSWORD
#                         always required — Harbor is always the push
#                         destination, regardless of source.

FAILED=()
SRC_INSPECT_FLAGS=()
SRC_COPY_FLAGS=()

if [[ "${MIRROR_REQUIRES_AUTH:-false}" == "true" ]]; then
  if [[ -z "${GHCR_USER:-}" || -z "${GHCR_TOKEN:-}" ]]; then
    echo "== skipping: GHCR_USER/GHCR_TOKEN not set =="
    exit 0
  fi
  echo "== logging in to ghcr.io =="
  skopeo login ghcr.io --username "${GHCR_USER}" --password "${GHCR_TOKEN}"
else
  SRC_INSPECT_FLAGS=(--no-creds)
  SRC_COPY_FLAGS=(--src-no-creds)
fi

mirror_one() {
  local src="$1" dest="$2"
  echo "  ${src} -> ${dest}"

  local src_digest dest_digest
  src_digest=$(skopeo inspect "${SRC_INSPECT_FLAGS[@]}" "docker://${src}" --format '{{.Digest}}' 2>/dev/null || true)
  dest_digest=$(skopeo inspect --tls-verify=false \
    --creds "${HARBOR_ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
    "docker://${HARBOR_HOST}/${dest}" --format '{{.Digest}}' 2>/dev/null || true)

  if [[ -n "${src_digest}" && "${src_digest}" == "${dest_digest}" ]]; then
    echo "    up-to-date, skipping"
    return 0
  fi

  if skopeo copy "${SRC_COPY_FLAGS[@]}" --dest-tls-verify=false \
    --dest-creds "${HARBOR_ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
    "docker://${src}" "docker://${HARBOR_HOST}/${dest}"; then
    echo "    done"
  else
    echo "    FAILED"
    FAILED+=("${src}")
  fi
}

if [[ -n "${MIRROR_LIST_FILE:-}" ]]; then
  while IFS='|' read -r src dest; do
    [[ -z "${src}" ]] && continue
    mirror_one "${src}" "${dest}"
  done < "${MIRROR_LIST_FILE}"
elif [[ -n "${MIRROR_LIST:-}" ]]; then
  while IFS='|' read -r src dest; do
    [[ -z "${src}" ]] && continue
    mirror_one "${src}" "${dest}"
  done <<< "${MIRROR_LIST}"
fi

if [[ ${#FAILED[@]} -gt 0 ]]; then
  echo "Failed:${FAILED[*]}" >&2
  exit 1
fi
