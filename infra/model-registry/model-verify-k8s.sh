#!/usr/bin/env bash
# ==============================================================
# model-verify-k8s.sh
# Interactive Harbor model verifier.
#
# 1. Opens a kubectl port-forward to Harbor (tears it down on exit).
# 2. Scans the models declared in models/<name>.yml and shows only
#    the ones that are actually present in Harbor.
# 3. Asks the user to pick one model to verify, or exit.
# 4. Runs a short-lived Kubernetes Job (in a dedicated namespace
#    that is deleted on exit) with three containers:
#      initContainer (oras):  pulls the OCI artifact from Harbor
#      container (alpine):    structural directory checks
#      container (infer):     safetensors / config / tokenizer check
#                             via infer.py injected through a ConfigMap
#
# Prerequisites:
#   - Docker (for ORAS + yq)
#   - kubectl configured for the target cluster
#   - env-vars/<name> file with HARBOR_USER and HARBOR_PASSWORD set
#
# Usage:
#   ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-verify-k8s.sh
#
# On-prem:
#   KUBECONFIG=~/.kube/rke2-cluster.yaml ENV_FILE=... MODELS_YML=... bash model-verify-k8s.sh
#
# Tips:
#   - Default timeout is 1800s. Override for unusually large models:
#       JOB_TIMEOUT=3600 ENV_FILE=... MODELS_YML=... bash model-verify-k8s.sh
#   - The pull volume is a PVC (default 60 Gi = compressed artifact + extracted model).
#     Override for larger models:
#       VERIFY_PVC_SIZE=100Gi ENV_FILE=... MODELS_YML=... bash model-verify-k8s.sh
#   - On first run the ORAS/Python images are pulled to the node; subsequent
#     runs are faster because the images are cached.
# ==============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "${SCRIPT_DIR}/logs"
LOG_FILE="${SCRIPT_DIR}/logs/model-verify-k8s-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "${LOG_FILE}") 2>&1

if [[ -z "${MODELS_YML:-}" ]]; then
  echo "ERROR: MODELS_YML is required. E.g.: MODELS_YML=<stack-name>.yml bash model-verify-k8s.sh" >&2
  exit 1
fi
[[ "${MODELS_YML}" != /* ]] && MODELS_YML="${SCRIPT_DIR}/models/${MODELS_YML}"
MODELS_YML="$(realpath "${MODELS_YML}")"

ORAS_IMAGE="${ORAS_IMAGE:-ghcr.io/oras-project/oras:v1.3.1}"
# Image used inside the Kubernetes Job (must be reachable from the cluster).
# On air-gapped on-prem clusters override this to the Harbor-hosted ORAS image:
#   ORAS_CLUSTER_IMAGE=harbor.harbor.svc.cluster.local/images-infra/oras-project/oras:v1.3.1
ORAS_CLUSTER_IMAGE="${ORAS_CLUSTER_IMAGE:-${ORAS_IMAGE}}"
YQ_IMAGE="${YQ_IMAGE:-linuxserver/yq:3.4.3}"
INFER_IMAGE="${INFER_IMAGE:-python:3.12-slim}"
ALPINE_IMAGE="${ALPINE_IMAGE:-alpine:3.20}"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_SVC="${HARBOR_SVC:-harbor}"
HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_HOST="localhost"
HARBOR_REGISTRY="${HARBOR_HOST}:${HARBOR_LOCAL_PORT}"

# Harbor address as seen from inside the cluster (ClusterIP — no port-forward needed for Jobs)
HARBOR_CLUSTER_HOST="${HARBOR_CLUSTER_HOST:-harbor.harbor}"
HARBOR_CLUSTER_PORT="${HARBOR_CLUSTER_PORT:-80}"
HARBOR_CLUSTER_REGISTRY="${HARBOR_CLUSTER_HOST}:${HARBOR_CLUSTER_PORT}"

VERIFY_NAMESPACE="${VERIFY_NAMESPACE:-model-verify}"
JOB_TIMEOUT="${JOB_TIMEOUT:-1800}"
# PVC size for the pull volume.  ORAS downloads the compressed blob to a temp file
# first, then extracts and deletes it.  Peak usage = compressed artifact + extracted
# model (both present simultaneously during extraction).  For deepseek (~23 GiB
# compressed, ~27 GiB extracted) peak is ~50 GiB — default is 60Gi to give headroom.
# Override for larger models, e.g.: VERIFY_PVC_SIZE=100Gi
VERIFY_PVC_SIZE="${VERIFY_PVC_SIZE:-60Gi}"
# StorageClass for the verification PVC. Empty string = cluster default.
# Set VERIFY_STORAGE_CLASS=hostpath on on-prem clusters — the script will
# automatically create and delete a matching PV using VERIFY_HOSTPATH_NODE
# and VERIFY_HOSTPATH_DIR.
VERIFY_STORAGE_CLASS="${VERIFY_STORAGE_CLASS:-}"
VERIFY_HOSTPATH_NODE="${VERIFY_HOSTPATH_NODE:-}"
VERIFY_HOSTPATH_DIR="${VERIFY_HOSTPATH_DIR:-/var/lib/hostpath/model-verify}"

# ── Load env ──────────────────────────────────────────────────────────────────
if [[ -z "${ENV_FILE:-}" ]]; then
  echo "ERROR: ENV_FILE is required. E.g.: ENV_FILE=<stack-name> bash model-verify-k8s.sh" >&2
  exit 1
fi
[[ "${ENV_FILE}" != /* ]] && ENV_FILE="${SCRIPT_DIR}/env-vars/${ENV_FILE}"
[[ -f "${ENV_FILE}" ]] && source "${ENV_FILE}"

# ── Logging ───────────────────────────────────────────────────────────────────
ts()      { date '+%H:%M:%S'; }
info()    { echo "$(ts) [INFO]  $*"; }
pass()    { echo "$(ts) [PASS]  $*"; }
fail()    { echo "$(ts) [FAIL]  $*"; }
warn()    { echo "$(ts) [WARN]  $*" >&2; }
error()   { echo "$(ts) [ERROR] $*" >&2; }
section() { echo ""; echo "$(ts) ──── $* ────"; }

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ -z "${HARBOR_USER:-}" || -z "${HARBOR_PASSWORD:-}" ]]; then
  error "HARBOR_USER and HARBOR_PASSWORD are required. Set them in the env file or as env vars."
  exit 1
fi
for cmd in kubectl docker; do
  if ! command -v "${cmd}" &>/dev/null; then
    error "${cmd} is required."
    exit 1
  fi
done

# ── Optional kubeconfig ───────────────────────────────────────────────────────
kubectl_args=()
if [[ -n "${KUBECONFIG:-}" ]]; then
  kubectl_args=(--kubeconfig "${KUBECONFIG}")
fi
kube() { kubectl "${kubectl_args[@]}" "$@"; }

# ── Cleanup trap ──────────────────────────────────────────────────────────────
PF_PID=""
CREDS_DIR=""
HOSTPATH_PV_NAME=""
cleanup() {
  echo ""
  [[ -n "${PF_PID}" ]] && kill "${PF_PID}" 2>/dev/null || true
  [[ -n "${CREDS_DIR}" ]] && rm -rf "${CREDS_DIR}"
  info "Deleting verification namespace (${VERIFY_NAMESPACE})..."
  kube delete namespace "${VERIFY_NAMESPACE}" --ignore-not-found 2>/dev/null || true
  if [[ -n "${HOSTPATH_PV_NAME}" ]]; then
    info "Deleting hostpath PV (${HOSTPATH_PV_NAME})..."
    kube delete pv "${HOSTPATH_PV_NAME}" --ignore-not-found 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ── Startup context ───────────────────────────────────────────────────────────
section "model-verify-k8s.sh"
info "Env file  : ${ENV_FILE}"
info "Models    : ${MODELS_YML}"
info "Kubectl   : $(kubectl "${kubectl_args[@]}" config current-context)"

# ── Port-forward ──────────────────────────────────────────────────────────────
section "Port-forward"
if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  info "Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → ${HARBOR_SVC}.${HARBOR_NAMESPACE}:80"
  kubectl "${kubectl_args[@]}" port-forward \
    -n "${HARBOR_NAMESPACE}" \
    "svc/${HARBOR_SVC}" \
    "${HARBOR_LOCAL_PORT}:80" \
    > "${SCRIPT_DIR}/logs/harbor-portforward-verify-k8s.log" 2>&1 &
  PF_PID=$!
  for i in $(seq 1 10); do
    if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
    sleep 1
  done
  if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
    error "Port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}."
    error "Check ${SCRIPT_DIR}/logs/harbor-portforward-verify-k8s.log for details."
    exit 1
  fi
  info "Port-forward ready."
else
  info "Port-forward already active on localhost:${HARBOR_LOCAL_PORT} — reusing."
fi

# ── ORAS login ────────────────────────────────────────────────────────────────
CREDS_DIR="$(mktemp -d)"
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

# ── yq wrapper ───────────────────────────────────────────────────────────────
yq() {
  docker run --rm -v "${MODELS_YML}:/models.yml:ro" "${YQ_IMAGE}" yq -r "$@" /models.yml \
    | awk '/\[ls\.io-init\] done\./{p=1; next} p'
}

# ── Scan Harbor for available models ─────────────────────────────────────────
section "Available models in Harbor"

model_count=$(yq '.models | length')

# Parallel arrays indexed by menu position (0-based internally)
MENU_YAML_IDX=()    # index into the YAML models array
MENU_IDS=()         # HuggingFace model ID
MENU_REFS=()        # full Harbor ref (cluster-side) used in the K8s Job
MENU_LOCAL_REFS=()  # full Harbor ref (local port-forward) used for manifest fetch

for i in $(seq 0 $((model_count - 1))); do
  model_id=$(yq ".models[${i}].id")
  harbor_project=$(yq ".models[${i}].harbor_project")
  harbor_name=$(yq ".models[${i}].harbor_name")
  harbor_tag=$(yq ".models[${i}].harbor_tag")

  local_ref="${HARBOR_REGISTRY}/${harbor_project}/${harbor_name}:${harbor_tag}"
  cluster_ref="${HARBOR_CLUSTER_REGISTRY}/${harbor_project}/${harbor_name}:${harbor_tag}"

  info "  checking ${model_id}..."
  if docker run --rm \
      --network host \
      -v "${CREDS_DIR}:/root/.docker" \
      "${ORAS_IMAGE}" \
      manifest fetch --plain-http "${local_ref}" >/dev/null 2>&1; then
    MENU_YAML_IDX+=("${i}")
    MENU_IDS+=("${model_id}")
    MENU_REFS+=("${cluster_ref}")
    MENU_LOCAL_REFS+=("${local_ref}")
  else
    info "  [ ] ${model_id}  — not in Harbor"
  fi
done

if [[ ${#MENU_IDS[@]} -eq 0 ]]; then
  error "No models found in Harbor. Run model-sync.sh first."
  exit 1
fi

# ── Interactive selection ─────────────────────────────────────────────────────
# Print the full menu to /dev/tty only after all checks are done so that
# tee-buffered output from the scan loop does not interleave with the menu.
{
  echo ""
  for j in "${!MENU_IDS[@]}"; do
    printf "  [%d] %s\n      %s\n" "$((j + 1))" "${MENU_IDS[$j]}" "${MENU_REFS[$j]}"
  done
  echo ""
  echo "  [0] Exit"
  echo ""
  printf "Choose a model to verify (0-%d): " "${#MENU_IDS[@]}"
} > /dev/tty
read -r choice < /dev/tty

if [[ -z "${choice}" || "${choice}" == "0" ]]; then
  info "Exiting."
  exit 0
fi

if ! [[ "${choice}" =~ ^[0-9]+$ ]] || \
   [[ "${choice}" -lt 1 || "${choice}" -gt ${#MENU_IDS[@]} ]]; then
  error "Invalid choice: ${choice}"
  exit 1
fi

SEL=$(( choice - 1 ))
SEL_YAML_IDX="${MENU_YAML_IDX[${SEL}]}"
SEL_MODEL_ID="${MENU_IDS[${SEL}]}"
SEL_CLUSTER_REF="${MENU_REFS[${SEL}]}"
SEL_LOCAL_REF="${MENU_LOCAL_REFS[${SEL}]}"

info "Selected  : ${SEL_MODEL_ID}"
info "Ref       : ${SEL_CLUSTER_REF}"

# ── OCI artifact size (sum of layer sizes from manifest) ─────────────────────
info "Fetching artifact size from Harbor..."
manifest_json=$(docker run --rm \
  --network host \
  -v "${CREDS_DIR}:/root/.docker" \
  "${ORAS_IMAGE}" \
  manifest fetch --plain-http "${SEL_LOCAL_REF}" 2>/dev/null || true)
if [[ -n "${manifest_json}" ]]; then
  total_bytes=$(echo "${manifest_json}" \
    | grep -o '"size": *[0-9]*' | grep -o '[0-9]*' \
    | awk '{sum+=$1} END{print sum+0}')
  artifact_size=$(awk -v b="${total_bytes}" 'BEGIN{
    if (b >= 1073741824) printf "%.2f GiB", b/1073741824
    else if (b >= 1048576) printf "%.2f MiB", b/1048576
    else printf "%.2f KiB", b/1024
  }')
  info "Artifact size : ${artifact_size}  (${total_bytes} bytes, compressed OCI layers)"
else
  warn "Could not fetch manifest for size calculation."
fi

# ── Collect expected directories ──────────────────────────────────────────────
model_dir_name="models--$(echo "${SEL_MODEL_ID}" | sed 's|/|--|g')"
expected_dirs=("${model_dir_name}")

dep_count=$(yq ".models[${SEL_YAML_IDX}].dependencies | length" 2>/dev/null || echo "0")
for j in $(seq 0 $((dep_count - 1))); do
  dep_id=$(yq ".models[${SEL_YAML_IDX}].dependencies[${j}].id")
  expected_dirs+=("models--$(echo "${dep_id}" | sed 's|/|--|g')")
done

# ── Namespace + ConfigMap ─────────────────────────────────────────────────────
section "Namespace"
if kube get namespace "${VERIFY_NAMESPACE}" &>/dev/null; then
  info "Namespace ${VERIFY_NAMESPACE} already exists — reusing."
else
  info "Creating namespace ${VERIFY_NAMESPACE}..."
  kube create namespace "${VERIFY_NAMESPACE}"
fi

INFER_CONFIGMAP="infer-script"
if kube get configmap "${INFER_CONFIGMAP}" -n "${VERIFY_NAMESPACE}" &>/dev/null; then
  info "ConfigMap ${INFER_CONFIGMAP} already exists — reusing."
else
  info "Creating ConfigMap ${INFER_CONFIGMAP}..."
  kube create configmap "${INFER_CONFIGMAP}" \
    --from-file=infer.py="${SCRIPT_DIR}/images/inferencer/infer.py" \
    -n "${VERIFY_NAMESPACE}"
fi

# ── Verify selected model ─────────────────────────────────────────────────────
section "Verifying ${SEL_MODEL_ID}"

TIMESTAMP=$(date '+%s')
safe_name="$(echo "${SEL_MODEL_ID}" | tr '[:upper:]' '[:lower:]' | tr '_/.' '-' | cut -c1-35)"
JOB_NAME="model-verify-${safe_name}-${TIMESTAMP}"
SECRET_NAME="harbor-creds-${safe_name}-${TIMESTAMP}"

info "Job       : ${JOB_NAME}"
info "Dirs      : ${expected_dirs[*]}"

# ── Create Harbor credentials Secret ─────────────────────────────────────────
info "[1/5] Creating credentials Secret..."
kube create secret docker-registry "${SECRET_NAME}" \
  --docker-server="${HARBOR_CLUSTER_REGISTRY}" \
  --docker-username="${HARBOR_USER}" \
  --docker-password="${HARBOR_PASSWORD}" \
  -n "${VERIFY_NAMESPACE}" > /dev/null

# ── Submit verification Job ───────────────────────────────────────────────────
PVC_NAME="${JOB_NAME}-pull"
info "[2/5] Creating PVC (${VERIFY_PVC_SIZE}) and Job..."
pvc_storage_class=""
if [[ -n "${VERIFY_STORAGE_CLASS}" ]]; then
  pvc_storage_class="  storageClassName: ${VERIFY_STORAGE_CLASS}"
fi

if [[ "${VERIFY_STORAGE_CLASS}" == "hostpath" ]]; then
  if [[ -z "${VERIFY_HOSTPATH_NODE}" ]]; then
    error "VERIFY_STORAGE_CLASS=hostpath requires VERIFY_HOSTPATH_NODE to be set."
    exit 1
  fi
  HOSTPATH_PV_NAME="${PVC_NAME}-pv"
  info "[2/5] Creating hostpath PV (${HOSTPATH_PV_NAME} → ${VERIFY_HOSTPATH_NODE}:${VERIFY_HOSTPATH_DIR})..."
  kube apply -f - > /dev/null <<EOF
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ${HOSTPATH_PV_NAME}
spec:
  capacity:
    storage: ${VERIFY_PVC_SIZE}
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: hostpath
  local:
    path: ${VERIFY_HOSTPATH_DIR}
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values:
                - ${VERIFY_HOSTPATH_NODE}
EOF
fi

kube apply -f - > /dev/null <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${PVC_NAME}
  namespace: ${VERIFY_NAMESPACE}
spec:
  accessModes:
    - ReadWriteOnce
${pvc_storage_class}
  resources:
    requests:
      storage: ${VERIFY_PVC_SIZE}
EOF
kube apply -f - > /dev/null <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  namespace: ${VERIFY_NAMESPACE}
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      volumes:
        - name: pull-dir
          persistentVolumeClaim:
            claimName: ${PVC_NAME}
        - name: oras-creds
          secret:
            secretName: ${SECRET_NAME}
            items:
              - key: .dockerconfigjson
                path: config.json
        - name: infer-script
          configMap:
            name: ${INFER_CONFIGMAP}
      initContainers:
        - name: mkdir-tmp
          image: ${ALPINE_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["mkdir", "-p", "/pull/.tmp"]
          volumeMounts:
            - name: pull-dir
              mountPath: /pull
        - name: pull
          image: ${ORAS_CLUSTER_IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: TMPDIR
              value: /pull/.tmp
          args:
            - pull
            - --plain-http
            - ${SEL_CLUSTER_REF}
          workingDir: /pull
          volumeMounts:
            - name: pull-dir
              mountPath: /pull
            - name: oras-creds
              mountPath: /root/.docker
      containers:
        - name: verify
          image: ${ALPINE_IMAGE}
          imagePullPolicy: IfNotPresent
          command:
            - sh
            - -c
            - |
              for dir in /pull/models--*/; do
                [ -d "\${dir}" ] || continue
                name="\${dir%/}"; name="\${name##*/}"
                echo "DIR_FOUND: \${name}"
                [ -d "\${dir}blobs" ]    && echo "BLOBS_OK: \${name}"      || echo "BLOBS_MISSING: \${name}"
                snap_count=\$(find "\${dir}snapshots" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
                [ "\${snap_count}" -gt 0 ] && echo "SNAPSHOTS_OK: \${name} (\${snap_count})" || echo "SNAPSHOTS_MISSING: \${name}"
                if [ -f "\${dir}refs/main" ]; then
                  hash=\$(cat "\${dir}refs/main")
                  [ -n "\${hash}" ] && echo "REFS_OK: \${name} \${hash}" || echo "REFS_EMPTY: \${name}"
                else
                  echo "REFS_MISSING: \${name}"
                fi
                disk=\$(du -sh "\${dir}" 2>/dev/null | cut -f1)
                disk_bytes=\$(du -sb "\${dir}" 2>/dev/null | cut -f1)
                echo "DISK_USAGE: \${name} \${disk}"
                echo "DISK_BYTES: \${name} \${disk_bytes}"
              done
              tmp_disk=\$(du -sh /pull/.tmp 2>/dev/null | cut -f1 || echo "0")
              echo "TEMP_USAGE: \${tmp_disk}"
          volumeMounts:
            - name: pull-dir
              mountPath: /pull
        - name: infer
          image: ${INFER_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/scripts/infer.py", "${model_dir_name}"]
          volumeMounts:
            - name: pull-dir
              mountPath: /pull
            - name: infer-script
              mountPath: /scripts
EOF

info "[3/5] Job created:"
kube -n "${VERIFY_NAMESPACE}" get job 2>/dev/null | sed 's/^/  /'

# ── Wait for Job ──────────────────────────────────────────────────────────────
info "[4/5] Waiting for Job (timeout: ${JOB_TIMEOUT}s)..."
job_ok=0
job_failed=0
elapsed=0
while [[ ${elapsed} -lt ${JOB_TIMEOUT} ]]; do
  job_json=$(kube get job/"${JOB_NAME}" -n "${VERIFY_NAMESPACE}" -o json 2>/dev/null || true)
  if [[ -z "${job_json}" ]]; then
    fail "${SEL_MODEL_ID} — Job disappeared unexpectedly (deleted while running?)"
    job_failed=1
    break
  fi
  succeeded=$(echo "${job_json}" | grep -o '"succeeded": *[0-9]*' | grep -o '[0-9]*' || echo "0")
  failed=$(echo "${job_json}"    | grep -o '"failed": *[0-9]*'    | grep -o '[0-9]*' || echo "0")
  if [[ "${succeeded}" -ge 1 ]]; then
    job_ok=1
    break
  fi
  if [[ "${failed}" -ge 1 ]]; then
    fail "${SEL_MODEL_ID} — Job failed"
    job_failed=1
    break
  fi
  sleep 5
  elapsed=$(( elapsed + 5 ))
done

if [[ ${job_ok} -eq 0 && ${job_failed} -eq 0 ]]; then
  fail "${SEL_MODEL_ID} — Job timed out after ${JOB_TIMEOUT}s"
  job_failed=1
fi

if [[ ${job_failed} -eq 1 ]]; then

  # ── Dump pod events (image-pull / scheduling problems show up here) ──────────
  echo ""
  info "  Pod events / status:"
  pod_name=$(kube get pods -n "${VERIFY_NAMESPACE}" \
    -l "job-name=${JOB_NAME}" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [[ -n "${pod_name}" ]]; then
    info "  Pod: ${pod_name}"
    kube describe pod "${pod_name}" -n "${VERIFY_NAMESPACE}" 2>/dev/null \
      | awk '/^Events:/,0' \
      | sed 's/^/    /' || true
  else
    warn "  No pod found for job ${JOB_NAME} — it may never have scheduled."
  fi

  # ── Container logs ───────────────────────────────────────────────────────────
  echo ""
  info "  initContainer (pull) logs:"
  kube logs -n "${VERIFY_NAMESPACE}" "job/${JOB_NAME}" -c pull 2>/dev/null | sed 's/^/    /' || true
  info "  verify logs:"
  kube logs -n "${VERIFY_NAMESPACE}" "job/${JOB_NAME}" -c verify 2>/dev/null | sed 's/^/    /' || true
  info "  infer logs:"
  kube logs -n "${VERIFY_NAMESPACE}" "job/${JOB_NAME}" -c infer 2>/dev/null | sed 's/^/    /' || true
  exit 1
fi

# ── Check results ─────────────────────────────────────────────────────────────
info "[5/5] Checking results..."
verify_logs=$(kube logs -n "${VERIFY_NAMESPACE}" "job/${JOB_NAME}" -c verify 2>/dev/null || true)

check_failed=0
for dir in "${expected_dirs[@]}"; do
  if echo "${verify_logs}" | grep -q "^DIR_FOUND: ${dir}$"; then
    info "  dir      : ${dir} — found"
  else
    fail "${SEL_MODEL_ID} — directory missing: ${dir}"
    check_failed=1
    continue
  fi

  if echo "${verify_logs}" | grep -q "^BLOBS_OK: ${dir}$"; then
    info "  blobs    : ${dir}/blobs — OK"
  else
    fail "${SEL_MODEL_ID} — blobs/ missing in ${dir}"
    check_failed=1
  fi

  if echo "${verify_logs}" | grep -q "^SNAPSHOTS_OK: ${dir}"; then
    snap_info=$(echo "${verify_logs}" | grep "^SNAPSHOTS_OK: ${dir}" | sed "s/^SNAPSHOTS_OK: ${dir} //")
    info "  snapshots: ${dir}/snapshots — OK (${snap_info})"
  else
    fail "${SEL_MODEL_ID} — no snapshot directory in ${dir}/snapshots"
    check_failed=1
  fi

  if echo "${verify_logs}" | grep -q "^REFS_OK: ${dir}"; then
    ref_hash=$(echo "${verify_logs}" | grep "^REFS_OK: ${dir}" | awk '{print $3}')
    info "  refs     : ${dir}/refs/main — ${ref_hash}"
  elif echo "${verify_logs}" | grep -q "^REFS_EMPTY: ${dir}$"; then
    fail "${SEL_MODEL_ID} — refs/main is empty in ${dir}"
    check_failed=1
  else
    fail "${SEL_MODEL_ID} — refs/main missing in ${dir}"
    check_failed=1
  fi

  disk=$(echo "${verify_logs}" | grep "^DISK_USAGE: ${dir} " | awk '{print $3}')
  [[ -n "${disk}" ]] && info "  on-disk  : ${dir} — ${disk}"
done

tmp_disk=$(echo "${verify_logs}" | grep "^TEMP_USAGE:" | awk '{print $2}')
[[ -n "${tmp_disk}" ]] && info "  oras tmp : /pull/.tmp — ${tmp_disk}"

# ── Peak PVC usage during extraction ─────────────────────────────────────────
total_disk_bytes=0
for dir in "${expected_dirs[@]}"; do
  dir_bytes=$(echo "${verify_logs}" | grep "^DISK_BYTES: ${dir} " | awk '{print $3}')
  [[ -n "${dir_bytes}" ]] && total_disk_bytes=$(( total_disk_bytes + dir_bytes ))
done
if [[ -n "${total_bytes:-}" && "${total_bytes}" -gt 0 && "${total_disk_bytes}" -gt 0 ]]; then
  peak_bytes=$(( total_bytes + total_disk_bytes ))
  peak_human=$(awk -v b="${peak_bytes}" 'BEGIN{
    if (b >= 1073741824) printf "%.2f GiB", b/1073741824
    else if (b >= 1048576) printf "%.2f MiB", b/1048576
    else printf "%.2f KiB", b/1024
  }')
  info "  peak PVC : ~${peak_human}  (${total_bytes} compressed + ${total_disk_bytes} extracted)"
fi

echo ""
info "  infer output:"
kube logs -n "${VERIFY_NAMESPACE}" "job/${JOB_NAME}" -c infer 2>/dev/null | sed 's/^/    /' || true

# ── Pull duration ─────────────────────────────────────────────────────────────
echo ""
pod_name_ok=$(kube get pods -n "${VERIFY_NAMESPACE}" \
  -l "job-name=${JOB_NAME}" \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
if [[ -n "${pod_name_ok}" ]]; then
  pull_start=$(kube get pod "${pod_name_ok}" -n "${VERIFY_NAMESPACE}" \
    -o jsonpath='{.status.initContainerStatuses[?(@.name=="pull")].state.terminated.startedAt}' \
    2>/dev/null || true)
  pull_end=$(kube get pod "${pod_name_ok}" -n "${VERIFY_NAMESPACE}" \
    -o jsonpath='{.status.initContainerStatuses[?(@.name=="pull")].state.terminated.finishedAt}' \
    2>/dev/null || true)
  if [[ -n "${pull_start}" && -n "${pull_end}" ]]; then
    pull_start_ts=$(date -d "${pull_start}" +%s 2>/dev/null || true)
    pull_end_ts=$(date -d "${pull_end}"   +%s 2>/dev/null || true)
    if [[ -n "${pull_start_ts}" && -n "${pull_end_ts}" ]]; then
      pull_secs=$(( pull_end_ts - pull_start_ts ))
      pull_min=$(( pull_secs / 60 ))
      pull_sec=$(( pull_secs % 60 ))
      info "  pull duration : ${pull_min}m${pull_sec}s  (started ${pull_start}, finished ${pull_end})"
    fi
  fi
fi

echo ""
if [[ ${check_failed} -eq 0 ]]; then
  pass "${SEL_MODEL_ID} — all checks passed"
else
  fail "${SEL_MODEL_ID} — one or more checks failed"
  exit 1
fi
