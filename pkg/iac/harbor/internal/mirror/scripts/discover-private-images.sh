set -eu
: > /images/private-images.txt

# GHCR_SYNC_MODE "pinned" never reaches this script at all — it runs as its
# own one-shot Job with a fixed list, see deployPinnedImagesJob, since it
# needs no discovery step whatsoever.
#
#   "min-version": for each "package|min_version" pair in min-versions.txt,
#   discovers every published tag for that package and keeps only tags
#   whose natural-sort order is >= min_version (via `sort -V`, i.e. GNU
#   version sort — this is a heuristic, not a strict semver parser; it
#   handles typical "v1.2.3"/"1.2.3" tags well but isn't spec-exact for
#   pre-release qualifiers). Packages absent from the list are skipped
#   entirely — no discovery fallback — so a run is deterministic.
#
#   "all" (default): every currently-published tag under
#   ghcr.io/${GHCR_ORG}/* for every org package, discovered dynamically.

if [ "${GHCR_SYNC_MODE:-all}" = "min-version" ]; then
  if [ -z "${GHCR_TOKEN:-}" ]; then
    echo "GHCR_TOKEN not set — skipping min-version discovery"
    exit 0
  fi

  apk add --no-cache curl jq coreutils >/dev/null

  auth_header="Authorization: Bearer ${GHCR_TOKEN}"
  accept_header="Accept: application/vnd.github+json"

  while IFS='|' read -r pkg min_version; do
    [ -z "${pkg}" ] && continue

    tags=$(curl -sf -H "${auth_header}" -H "${accept_header}" \
      "https://api.github.com/orgs/${GHCR_ORG}/packages/container/${pkg}/versions?per_page=100" \
      | jq -r '.[].metadata.container.tags[]? | select(startswith("sha256-") | not)')

    for tag in ${tags}; do
      highest=$(printf '%s\n%s\n' "${min_version}" "${tag}" | sort -V | tail -1)
      if [ "${highest}" = "${tag}" ]; then
        echo "ghcr.io/${GHCR_ORG}/${pkg}:${tag}|images-shaide/${pkg}:${tag}" >> /images/private-images.txt
      fi
    done
  done < /images-static/min-versions.txt

  echo "Discovered $(wc -l < /images/private-images.txt) private image tag(s) >= pinned minimums:"
  cat /images/private-images.txt
  exit 0
fi

if [ -z "${GHCR_TOKEN:-}" ]; then
  echo "GHCR_TOKEN not set — skipping private image discovery"
  exit 0
fi

apk add --no-cache curl jq >/dev/null

auth_header="Authorization: Bearer ${GHCR_TOKEN}"
accept_header="Accept: application/vnd.github+json"

packages=$(curl -sf -H "${auth_header}" -H "${accept_header}" \
  "https://api.github.com/orgs/${GHCR_ORG}/packages?package_type=container&per_page=100" \
  | jq -r '.[].name')

for pkg in ${packages}; do
  tags=$(curl -sf -H "${auth_header}" -H "${accept_header}" \
    "https://api.github.com/orgs/${GHCR_ORG}/packages/container/${pkg}/versions?per_page=100" \
    | jq -r '.[].metadata.container.tags[]? | select(startswith("sha256-") | not)')

  for tag in ${tags}; do
    echo "ghcr.io/${GHCR_ORG}/${pkg}:${tag}|images-shaide/${pkg}:${tag}" >> /images/private-images.txt
  done
done

echo "Discovered $(wc -l < /images/private-images.txt) private image tag(s):"
cat /images/private-images.txt
