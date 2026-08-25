#!/usr/bin/env bash
# ==============================================================
# harbor-setup.sh
# Creates Harbor projects and robot account via the Harbor REST
# API.
#
# Run ONCE after the first successful `pulumi up` that deploys
# Harbor. Then re-run `pulumi up` to create the harbor-pull-secret
# (harbor:robotPassword is already in Pulumi stack config).
#
# Prerequisites:
#   - Harbor is running (pulumi up completed)
#   - kubectl configured for the GCP cluster
#   - harbor:adminPassword and harbor:robotPassword set in Pulumi
#     stack config (robotPassword must contain a special character)
#
# Usage:
#   HARBOR_ADMIN_PASSWORD=$(pulumi config get harbor:adminPassword) \
#   HARBOR_ROBOT_PASSWORD=$(pulumi config get harbor:robotPassword) \
#   bash scripts/harbor-setup.sh
#
# Interactively prompts for project visibility (private/public) when run at a
# TTY. Set HARBOR_PROJECT_VISIBILITY=public|private to skip the prompt (CI,
# scripted runs); with no TTY and no override, defaults to private.
#
# After this script completes:
#   pulumi up   # creates harbor-pull-secret using harbor:robotPassword
# ==============================================================
set -euo pipefail

HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_API="http://localhost:${HARBOR_LOCAL_PORT}/api/v2.0"
ROBOT_SECRET_FILE="k8s-harbor-sa-secret"

if [[ -z "${HARBOR_ADMIN_PASSWORD:-}" ]]; then
  echo "ERROR: HARBOR_ADMIN_PASSWORD env var is required." >&2
  exit 1
fi

if [[ -z "${HARBOR_ROBOT_PASSWORD:-}" ]]; then
  echo "ERROR: HARBOR_ROBOT_PASSWORD env var is required." >&2
  exit 1
fi

# ── Port-forward ────────────────────────────────────────────────────────────────
echo "==> Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → harbor svc/harbor:80"
kubectl port-forward -n harbor svc/harbor "${HARBOR_LOCAL_PORT}:80" &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true' EXIT

# Give port-forward a moment to establish
sleep 3

# ── Remove default library project ───────────────────────────────────────────
echo "==> Removing default 'library' project..."
http_code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -X DELETE "${HARBOR_API}/projects/library")

if [[ "${http_code}" == "200" ]]; then
  echo "    Deleted 'library' project."
elif [[ "${http_code}" == "404" ]]; then
  echo "    'library' project not found — skipping."
elif [[ "${http_code}" == "412" ]]; then
  echo "    'library' project has repositories — skipping deletion."
else
  echo "    WARNING: Could not delete 'library' project (HTTP ${http_code})."
fi

# ── Project visibility ──────────────────────────────────────────────────────
# HARBOR_PROJECT_VISIBILITY=public|private skips the menu for scripted/CI runs.
if [[ -n "${HARBOR_PROJECT_VISIBILITY:-}" ]]; then
  case "${HARBOR_PROJECT_VISIBILITY}" in
    public|private) visibility="${HARBOR_PROJECT_VISIBILITY}" ;;
    *)
      echo "ERROR: HARBOR_PROJECT_VISIBILITY must be 'public' or 'private' (got '${HARBOR_PROJECT_VISIBILITY}')." >&2
      exit 1
      ;;
  esac
  echo "==> Project visibility: ${visibility} (from HARBOR_PROJECT_VISIBILITY)"
elif [[ -t 0 ]]; then
  echo "==> Select visibility for the projects about to be created (ai-models, images-shaide, images-infra):"
  PS3="Visibility [1-2]: "
  select opt in "private (pulls require auth)" "public (anonymous pulls allowed)"; do
    case "${REPLY}" in
      1) visibility="private"; break ;;
      2) visibility="public"; break ;;
      *) echo "Enter 1 or 2." ;;
    esac
  done
else
  echo "==> No TTY and HARBOR_PROJECT_VISIBILITY not set — defaulting to private." >&2
  visibility="private"
fi

project_public="false"
[[ "${visibility}" == "public" ]] && project_public="true"

# ── Projects ─────────────────────────────────────────────────────────────────
echo "==> Creating Harbor projects (${visibility})..."

for project in ai-models images-shaide images-infra; do
  http_code=$(curl -s -o /dev/null -w "%{http_code}" \
    -u "admin:${HARBOR_ADMIN_PASSWORD}" \
    -X POST "${HARBOR_API}/projects" \
    -H "Content-Type: application/json" \
    -d "{\"project_name\":\"${project}\",\"public\":${project_public}}")

  if [[ "${http_code}" == "201" ]]; then
    echo "    Created project '${project}'."
  elif [[ "${http_code}" == "409" ]]; then
    echo "    Project '${project}' already exists — updating visibility to ${visibility}..."
    patch_code=$(curl -s -o /dev/null -w "%{http_code}" \
      -u "admin:${HARBOR_ADMIN_PASSWORD}" \
      -X PUT "${HARBOR_API}/projects/${project}" \
      -H "Content-Type: application/json" \
      -d "{\"metadata\":{\"public\":\"${project_public}\"}}")
    if [[ "${patch_code}" == "200" ]]; then
      echo "    Updated '${project}' to ${visibility}."
    else
      echo "    WARNING: Could not update '${project}' visibility (HTTP ${patch_code})." >&2
    fi
  else
    echo "ERROR: Failed to create project '${project}' (HTTP ${http_code})." >&2
    exit 1
  fi
done

# ── Robot account ────────────────────────────────────────────────────────────
echo "==> Checking for existing robot account 'robot\$k8s-harbor-sa'..."

existing=$(curl -s \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  "${HARBOR_API}/robots" \
  | grep -c '"robot$k8s-harbor-sa"' || true)

if [[ "${existing}" -gt 0 ]]; then
  echo "    Robot 'robot\$k8s-harbor-sa' already exists — skipping."
  echo ""
  echo "    If you need to re-create it, delete it via the Harbor UI first."
  exit 0
fi

echo "==> Creating robot account 'k8s-harbor-sa'..."

response=$(curl -s -w "\n%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -X POST "${HARBOR_API}/robots" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "k8s-harbor-sa",
    "level": "system",
    "duration": -1,
    "permissions": [
      {
        "kind": "project",
        "namespace": "*",
        "access": [
          {"action": "pull",  "resource": "repository"},
          {"action": "push",  "resource": "repository"},
          {"action": "read",  "resource": "artifact"}
        ]
      }
    ]
  }')

http_code=$(echo "${response}" | tail -1)
body=$(echo "${response}" | head -n -1)

if [[ "${http_code}" != "201" ]]; then
  echo "ERROR: Failed to create robot account (HTTP ${http_code})." >&2
  echo "${body}" >&2
  exit 1
fi

robot_id=$(echo "${body}" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)

if [[ -z "${robot_id}" ]]; then
  echo "ERROR: Could not extract robot ID from response." >&2
  echo "${body}" >&2
  exit 1
fi

echo "==> Setting robot secret (ID: ${robot_id}) via PATCH..."
http_code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -X PATCH "${HARBOR_API}/robots/${robot_id}" \
  -H "Content-Type: application/json" \
  -d "{\"secret\":\"${HARBOR_ROBOT_PASSWORD}\"}")

if [[ "${http_code}" != "200" ]]; then
  echo "WARNING: PATCH returned HTTP ${http_code} — secret may not be set correctly." >&2
fi

echo "==> Verifying robot credentials against registry token service..."
http_code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "robot\$k8s-harbor-sa:${HARBOR_ROBOT_PASSWORD}" \
  "http://localhost:${HARBOR_LOCAL_PORT}/service/token?service=harbor-registry")

if [[ "${http_code}" == "200" ]]; then
  echo "    Credentials verified successfully."
else
  echo "WARNING: Credential check returned HTTP ${http_code}." >&2
  echo "         Run harbor-reset-robot-secret.sh to retry." >&2
fi

echo ""
echo "==> Robot account created."
echo ""
echo "    Next steps:"
echo "    1. Run pulumi up to create the harbor-pull-secret in the harbor namespace."
echo "    2. Register the robot password with the app-serving stack:"
echo "         cd ../../../app_serving/deployments"
echo "         pulumi config set --secret app-serving:harborToken \$(pulumi config get harbor:robotPassword --show-secrets --cwd $(pwd)/..) --stack <stack-name>"
