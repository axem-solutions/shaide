# shaide documentation - proposed outline

**Status: implemented.** All chapters below exist under `docs/`. This file remains the
source map - it records where each page came from, which is what you need when the
private repo is eventually archived.

Working document for the docs consolidation. Maps every proposed page to its existing
source material across the public (`shaide`) and private (`shaide_ai_platform_private`)
repositories, and flags what has to be written from scratch.

**Legend**
`[public]` source lives in `shaide` · `[private]` source lives in `shaide_ai_platform_private`
**NEW** must be written · **MERGE** several sources combine into one page

---

## Proposed structure

```
docs/
├── index.md
├── introduction/
├── cluster-requirements/
├── cluster-setup/
├── getting-started/
├── installation/
├── usage/
├── architecture/
├── operations/
├── reference/
├── contributing/
└── adr/
```

Numeric prefixes are applied at build time by the nav manifest (see *Tooling notes*),
not baked into filenames - Framer URLs stay clean and pages can be reordered without
breaking links.

---

## 1. Introduction

| Page | Source | Notes |
| --- | --- | --- |
| `what-is-shaide.md` | `README.md` [public] | Value proposition, sovereignty, target audience. Adapted from the README. |
| `architecture-overview.md` | `README.md`, `docs/assets/architecture.png` [public] | One-page mental model before any detail. |
| `concepts.md` | — | **NEW.** Glossary: model service, inference pool, GAIE/EPP, ModelService, bundle, stack, nodegroup. |
| `deployment-targets.md` | `infra/*/[AWS\|GCP\|AZURE\|RKE2].md` [public] | Decision page: which target, what each implies. |

## 2. Getting started

| Page | Source | Notes |
| --- | --- | --- |
| `prerequisites.md` | `installer/docs/customer/prerequisites.md` (170) [public]<br>`infra/on-prem/documentation/PREREQUISITES.md` (375) [private] | **MERGE.** The general contract; per-target specifics live in *Cluster setup*. |
| `quickstart.md` | `README.md`, `installer/docs/customer/installation-guide.md` [public] | Shortest path to a serving platform. |
| `verify-installation.md` | `installer/docs/development/CHECKLIST_INSTALLER.md` (344) [public] | Post-install health checks. |

## 3. Cluster requirements *(separate chapter)*

A clean statement of what a target cluster must provide - resources and parameters only,
no build instructions.

| Page | Source | Notes |
| --- | --- | --- |
| `index.md` | `installer/docs/customer/prerequisites.md` [public]<br>`infra/on-prem/documentation/PREREQUISITES.md` [private] | **NEW/MERGE.** Summary table. |
| `compute.md` | as above | Nodes, GPUs, drivers, VRAM sizing. |
| `storage.md` | as above | StorageClass and capacity. |
| `networking.md` | as above | Ingress, ports, DNS/TLS, egress. |
| `verification.md` | **NEW** | Six commands that validate a cluster. |

## 4. Cluster setup

One guide per target. Build instructions only - requirements live in the chapter above.

| Page | Source | Notes |
| --- | --- | --- |
| `aws-eks.md` | `infra/aws/AWS.md` (115) [public]<br>`infra/aws/README.md` (343) [private] | Guide exists; enrich from the private README. |
| `gcp-gke.md` | `infra/gcp/GCP.md` (114) [public]<br>`infra/gcp/README.md` (231), `documentation/src/gcp.md` (53) [private] | Add Workload Identity material. |
| `azure-aks.md` | `infra/azure/AZURE.md` (118) [public]<br>`infra/azure/{README,03_cluster/README,03_cluster/QUOTA,03_cluster/AKS_ACCESS}.md` [private] | Private Azure docs total ~2,400 lines across the phased layout. **Harvest, do not port wholesale** - that phased structure is being retired. QUOTA.md is genuinely valuable. |
| `on-prem-rke2.md` | `infra/on-prem/RKE2.md` (147) [public]<br>`infra/on-prem/documentation/{STEP_BY_STEP_INST_CMD,CHECKLIST,CONFIG_OPTIONS}.md`, `ansible/` [private] | Ansible project is the base, as requested: 13 roles, 8 playbooks. |
| `rke2-cni.md` | `documentation/src/CNI.md` (190) [private] | CNI selection and configuration. |

## 4. Installation

| Page | Source | Notes |
| --- | --- | --- |
| `installer-guide.md` | `installer/README.md` (833), `installer/docs/customer/installation-guide.md` (380) [public] | **MERGE.** The main install walkthrough. |
| `bundle.md` | `installer/README.md`, `installer/docs/development/BUILD_INSTALLER_BUNDLE.md` (112) [public] | Bundle contract, manifests, building one. Flagged for removal later - keep isolated so it is cheap to delete. |
| `configuration.md` | `installer/docs/development/STACK_FILES_CONFIG.md` (291) [public] | Stack config reference. |
| `air-gapped.md` | `infra/cloud-harbor/documentation/{IMAGE_MIRROR,NODE_REGISTRY_CONFIG}.md` [public] | **NEW/MERGE.** Currently scattered. This is a headline differentiator and deserves one authoritative page. |
| `troubleshooting.md` | `infra/local-k8s/documentation/TROUBLESHOOTING.md` (133) [private] | **Largely NEW** - existing content is local-dev only. |

## 5. Usage

**The largest gap in the entire documentation set.** Almost nothing exists today beyond
the README snippet.

| Page | Source | Notes |
| --- | --- | --- |
| `openai-api.md` | `README.md` [public] | **NEW.** Endpoint, auth, base URL, compatibility scope. |
| `chat-completions.md` | — | **NEW.** curl + SDK, streaming, parameters. |
| `embeddings.md` | `app_serving/README.md` [public] | **NEW.** Note the separate non-gateway path. |
| `agent-integrations.md` | — | **NEW.** LangChain, LlamaIndex, Claude Code, Continue, etc. Directly supports the "built for agent fleets" claim. |
| `model-catalog.md` | `app_serving/README.md` [public] | Validated models, GPU requirements. |
| `authentication.md` | — | **NEW.** Lives in `shaide_server`; needs at least a pointer page. |

## 6. Architecture

| Page | Source | Notes |
| --- | --- | --- |
| `overview.md` | `README.md` [public] | Layers and deployment order. |
| `platform-services.md` | `infra/cloud-harbor/README.md` (361) [public] | Harbor as image + model registry. |
| `gateway.md` | `infra/gateway-provider/{README,DESIGN,TLS,ENVOY}.md` (~650) [public]<br>`documentation/src/gateway-provider.md` (193) [private] | **MERGE.** Istio, Gateway API, GAIE. |
| `model-serving.md` | `app_serving/README.md` (495), `MODELS_DEPLOYMENT_FLOW.md` (556), `MODEL_STORAGE.md` (233) [public] | vLLM + llm-d, ModelService, storage. |
| `application-layer.md` | `app_shaide/README.md` (321), `deployments/README.md` (283) [public] | shaide server, control panel. |
| `mcp.md` | `app_mcp/README.md` (226) [public] | MCP datasources, NetworkPolicies. |
| `observability.md` | `monitoring/README.md` (885) [public] | Largest single doc in the repo. Loki/Grafana/Alloy/Prometheus. |
| `traffic-flow.md` | `documentation/src/infra-architecture-blueprints/*` [private] | Per-target blueprints - AWS, Azure, GCP, on-prem, GHCR. |

## 7. Operations

| Page | Source | Notes |
| --- | --- | --- |
| `model-management.md` | `app_serving/README.md`, `MODELS_DEPLOYMENT_FLOW.md` [public] | Adding, swapping, removing models. |
| `model-registry.md` | `infra/model-registry/{README,documentation/MODEL-REGISTRY-GUIDE}.md` (580) [public] | HF → Harbor as OCI artifacts. |
| `image-mirroring.md` | `infra/cloud-harbor/documentation/IMAGE_MIRROR.md` [public] | |
| `tls-certificates.md` | `infra/gateway-provider/TLS.md` (194) [public] | |
| `storage.md` | `documentation/src/kubernetes-pvc-resize.md` (66) [private] | PVC resize; extend with capacity planning. |
| `scaling.md` | — | **NEW.** Replicas, GPU allocation, single-GPU swaps. |
| `upgrades.md` | `infra/on-prem/ansible/roles/rke2_upgrade` [private] | **Largely NEW.** Platform upgrade + rollback. |
| `backup-restore.md` | — | **NEW.** State, Harbor, SQLite, Qdrant. |
| `azure-b2b-login.md` | `documentation/src/azure-login/AZURE-LOGIN.md` (294) [private] | Has screenshots. |
| `troubleshooting.md` | — | **NEW.** Symptom-indexed hub. |

## 8. Reference

| Page | Source | Notes |
| --- | --- | --- |
| `configuration.md` | `installer/docs/development/STACK_FILES_CONFIG.md` [public]<br>`infra/on-prem/documentation/CONFIG_OPTIONS.md` (636) [private] | **MERGE.** Full config key reference. |
| `ports.md` | `app_shaide/README.md`, `infra/local-k8s/documentation/PORTS.md` [private] | Consolidated port/network matrix. |
| `cli.md` | `installer/README.md` [public] | Installer flags and env vars. |
| `glossary.md` | — | **NEW.** |

## 9. Contributing

| Page | Source | Notes |
| --- | --- | --- |
| `development-setup.md` | `CONTRIBUTING.md` (122) [public] | |
| `local-cluster.md` | `infra/local-k8s/README.md` (481) + 10 docs (~1,100) [private] | vind-based local k8s. Excellent material, currently private-only. |
| `benchmarking.md` | `benchmarking/README.md` (176) [private] | llm-d-benchmark. |
| `release-process.md` | `RELEASE.md` (41) [public] | |
| `ai-policy.md` | `.github/AI_POLICY.md` (79) [public] | |

## 10. ADRs

| Page | Source |
| --- | --- |
| `001-centralized-logging.md` | `documentation/src/adr/001-centralized-logging.md` (123) [private] |

Keep the directory and the numbering convention; only one ADR exists so far.

---

## Suggestions beyond the original brief

1. **`cluster-setup/requirements.md`** - the single most valuable new page. A
   platform-agnostic requirements contract turns "runs anywhere Kubernetes runs" from a
   claim into a checklist, and it is what lets you delete the cloud-provider directories
   from `infra/` without losing the ability to onboard a new target.
2. **`usage/agent-integrations.md`** - the "built for agent fleets" positioning has no
   supporting documentation anywhere. Concrete integrations are the highest-leverage
   adoption content you can write.
3. **`installation/air-gapped.md`** - your strongest differentiator is currently
   documented only as scattered mirroring notes.
4. **`operations/troubleshooting.md`** - symptom-indexed, not component-indexed.
5. **`introduction/concepts.md`** - the stack layers vocabulary (GAIE, EPP,
   InferencePool, ModelService) is assumed everywhere and defined nowhere.

## Explicitly excluded

Stale or internal-only, do not migrate: `a.md`, `pkg/Refactor.md`,
`monitoring/_OLD/doc.md`, `documentation/src/_OLD/architecture.md`, `demo/README.md`,
`benchmarking/cae/test.md`, `.claude/commands/release.md`, `CHANGELOG.md`,
`infra/azure/0*/README.md` (harvest only - the phased layout is being retired),
`infra/on-prem/documentation/CAE_GEMMA_MODEL_SWAP.md` (customer-specific).

Also excluded per product scope: Knowledge Center, agents, and any RAG framing.

## Tooling notes

Constraints from Framer upload + an offline renderer that is not yet chosen:

- **One `# H1` per file**, matching the page title. Both Framer and every static site
  generator key off this.
- **Nav lives in one manifest** (`docs/nav.yml` or similar), not encoded in filenames or
  an mdBook-specific `SUMMARY.md`. Keeps you portable between mdBook, Docusaurus,
  MkDocs and Framer.
- **Relative links between docs**, always with the `.md` extension, so they resolve both
  on GitHub and after conversion.
- **Assets under `docs/assets/`**, referenced relatively. Compress before committing -
  `architecture.png` is currently 3.4 MB.
- **No renderer-specific syntax** in page bodies: no mdBook `{{#include}}`, no Docusaurus
  MDX/JSX, no GitHub `[!NOTE]` alerts (they render as plain blockquotes elsewhere).
- **Front-matter block** on every page (`title`, `description`, `weight`) - Framer can
  consume it for metadata and static generators for ordering.

## Migration sequencing

1. Scaffold `docs/` with the tree and front-matter stubs.
2. Move the highest-value public docs that need no rewriting (architecture, installer).
3. Harvest private-only material - Azure, on-prem, local-k8s.
4. Merge the multi-source pages (prerequisites, gateway, configuration).
5. Write the gaps, **Usage first** - it is both the largest hole and the most
   adoption-critical section.
