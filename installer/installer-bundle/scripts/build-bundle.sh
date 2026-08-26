#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [[ -f "$script_dir/../../build/Dockerfile" ]]; then
  # Scripts intentionally live inside installer/installer-bundle/scripts.
  repo_root=$(cd "$script_dir/../../.." && pwd)
else
  printf 'error: cannot locate installer/build/Dockerfile from %s\n' "$script_dir" >&2
  exit 1
fi

default_staging_dir="$repo_root/installer/installer-bundle/bundle"
default_output_archive="$repo_root/installer/installer-bundle/bundle.tar.gz"

usage() {
  cat <<'EOF'
Usage:
  build-bundle.sh [STAGING_DIRECTORY] [OUTPUT_ARCHIVE]

Inputs:
  STAGING_DIRECTORY  Bundle root containing deployments/, images/, and manifests/.
                     Defaults to installer/installer-bundle/bundle.
  OUTPUT_ARCHIVE     Destination .tar.gz file. Defaults to
                     installer/installer-bundle/bundle.tar.gz.

Example bundle:
  ./installer/installer-bundle/scripts/build-bundle.sh \
    installer/installer-bundle/example \
    installer/installer-bundle/example.tar.gz
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$#" -gt 2 ]]; then
  printf 'error: expected at most two inputs, got %d\n\n' "$#" >&2
  usage >&2
  exit 1
fi

staging_dir=${1:-"$default_staging_dir"}
output_archive=${2:-"$default_output_archive"}

if [[ ! -d "$staging_dir" ]]; then
  printf 'error: staging directory does not exist: %s\n\n' "$staging_dir" >&2
  printf 'The staging directory must contain:\n' >&2
  printf '  %s/deployments\n' "$staging_dir" >&2
  printf '  %s/images\n' "$staging_dir" >&2
  printf '  %s/manifests\n\n' "$staging_dir" >&2
  printf 'For the checked-in example, run:\n' >&2
  printf '  %s installer/installer-bundle/example installer/installer-bundle/example.tar.gz\n' \
    "$script_dir/build-bundle.sh" >&2
  exit 1
fi

for required in deployments images manifests; do
  if [[ ! -d "$staging_dir/$required" ]]; then
    printf 'error: missing required directory: %s\n\n' "$staging_dir/$required" >&2
    printf 'The staging directory must contain deployments/, images/, and manifests/.\n' >&2
    printf 'Run %s --help for the full input contract.\n' "$script_dir/build-bundle.sh" >&2
    exit 1
  fi
done

for required in images.yaml models.yaml; do
  if [[ ! -f "$staging_dir/manifests/$required" ]]; then
    printf 'error: missing required manifest: %s\n\n' "$staging_dir/manifests/$required" >&2
    printf 'Both manifests/images.yaml and manifests/models.yaml are required.\n' >&2
    printf 'Run %s --help for the full input contract.\n' "$script_dir/build-bundle.sh" >&2
    exit 1
  fi
done

if find "$staging_dir" -type l -print -quit | grep -q .; then
  printf 'error: symlinks are not supported in a bundle\n' >&2
  exit 1
fi

mkdir -p "$(dirname "$output_archive")"

# Hash the payload using paths relative to the bundle root.
payload_hash=$(
  cd "$staging_dir"
  find deployments images manifests -type f -print0 \
    | sort -z \
    | xargs -0 sha256sum \
    | sha256sum \
    | awk '{print $1}'
)

printf '{"sha256":"%s"}\n' "$payload_hash" > "$staging_dir/checksum.json"

temporary_archive=$(mktemp "${TMPDIR:-/tmp}/shaide-bundle.XXXXXX.tar.gz")
cleanup() { rm -f "$temporary_archive"; }
trap cleanup EXIT

# checksum.json must be the first tar entry; the extractor depends on this.
tar -C "$staging_dir" -czf "$temporary_archive" \
  checksum.json deployments images manifests

if [[ "$(tar -tzf "$temporary_archive" | awk 'NR == 1 { print }')" != "checksum.json" ]]; then
  printf 'error: checksum.json is not the first archive entry\n' >&2
  exit 1
fi

mv -f "$temporary_archive" "$output_archive"
trap - EXIT
printf 'created %s\n' "$output_archive"
