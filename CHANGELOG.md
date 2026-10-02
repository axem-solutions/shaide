# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project follows Semantic Versioning.

## [1.0.0] - 2026-10-02

### Added
- installer: assign node pools to models instead of single nodes
- installer: assign nodes to per-model pools in the TUI
- installer: select models from the packaged catalog in the TUI
- app-mcp: move the stack to pkg/iac and add framework scaffolding
- app-shaide: stack framework integration
- app-serving: stack framework integration with typed configuration entries and setters
- monitoring: stack framework integration with typed configuration entries and setters
- gateway-provider: stack framework integration
- harbor: stack framework integration with typed configuration entries and setters, deployed through a generic stack deployer
- installer: stack framework with typed configuration entries, setters, policies and prompts that resolve installer inputs into Pulumi configuration
- control-panel: add the KNOWLEDGE_CENTER_ENABLED environment variable
- replace nodeSelector with nodeAffinity across app_serving, app_shaide and app_mcp
- app_shaide: add the SESSION_SECRET environment variable to the control panel
- app_shaide: add the JWT_SECRET environment variable
- installer: package the Pulumi projects, manifests and the Harbor and monitoring Helm charts into the installer image

### Fixed
- installer: release model pods left on deleted nodes
- installer: never shrink an existing model volume
- installer: push model artifacts without a platform selection
- installer: keep the model caches in step with the pinned revision
- installer: say that the provisioning host's storage is checked
- installer: name storage files relative to the storage directory
- installer: verify a stack's cluster from recorded object UIDs
- gateway: let the Istio sweep tell this stack's objects from other Pulumi deployments
- installer: create missing Harbor projects in update mode
- gateway: pick the gateway class from the cluster and gate AGC on it
- installer: refuse to deploy a stack recorded against another cluster
- stack: trim whitespace from prompted input answers
- installer: pick the monitoring storage class from the cluster
- monitoring: auto-name the Loki bucket Job
- iac: deploy app-shaide and app-mcp to the selected context
- installer: skip app-serving when no models are selected
- installer: mirror only the target cluster's architecture
- installer: validate model manifest entries at startup
- monitoring: honor cluster storage defaults and GPU isolation
- installer: align images and admin credentials with deployed services
- installer: configure Harbor pulls on managed cluster nodes
- app-serving: make GPU deployment updates reliable
- app-serving: disable the deprecated routing proxy
- gateway-provider: stop the ownership sweep from deleting Istio on every run
- app-serving: bundle the pinned llm-d-infra chart in the installer image
- installer: pass the registry address to the app-serving stack
- installer: bound what a model transfer takes of the machine
- installer: add the llm-d chart to the Dockerfile and set harborRegistryHostName
- gateway-provider: ask for the gateway class instead of assuming it
- gateway-provider: keep the standalone Pulumi entry point exported
- installer: restart the blob upload when its session has expired
- installer: make large image uploads survive a dropped port-forward
- add a dedicated error for upstream registry problems
- app_mcp: fix a broken import
- harbor: check the pull secret after deploy, so stale secrets after a redeploy are caught
- harbor: integrate Harbor into the installer and fix the pull secret name mismatch
- app-serving: remove the llm-d routing sidecar

### Changed
- installer: pin the shaide server and control panel images to v1.0.0
- release: port the agentic release workflow; the installer image is also published as `latest`
- app-serving: move installer logic into the stack
- app-shaide: move installer logic into the stack
- gateway: document Istio as the shared gateway on AKS
- deps: record transitive module metadata checksums
- harbor: remove legacy setup scripts and mirroring instructions
- app-serving: extract model deployment orchestration
- build: raise the remaining modules to Go 1.26.6
- installer, pkg: upgrade Go from 1.26.3 to 1.26.6
- harbor: delete unused files
- docs: fix the Discord invite link
- gateway-provider: move deployment behaviour out of the installer into the gateway stack
- gateway-provider: restructure the gateway package into internal components
- architecture docs: describe nodeAffinity instead of nodeSelector in pod-level docs
- installer: unify the Harbor deployments into one stack
- harbor: merge cloud-harbor and onprem-harbor into a shared deployment
- harbor: move cloud-harbor from infra to the repository root
- serving, shaide: replace the vllm-config.json export with Kubernetes and vLLM discovery groundwork
- build: label the installer image with its source repository
- ci: publish the documentation to GitHub Pages
- docs: rename the prerequisites page to Provisioner Prerequisites
- docs: add the MkDocs site configuration
- gitignore build artifacts
- harbor: move cloud-harbor to harbor and unify the deploy path
- docs: collect the documentation
- docs: add a new README
