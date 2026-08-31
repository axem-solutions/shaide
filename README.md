<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/shaide_logo_white.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/shaide_logo_black.svg">
  <img alt="shaide" src="docs/assets/shaide_logo_black.svg" width="360">
</picture>

# The sovereign AI platform for enterprise

Distributed, multi-model LLM inference on Kubernetes you own -
installed by a single command, all the way down to air-gapped clusters.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/axem-solutions/shaide)](https://github.com/axem-solutions/shaide/releases)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Kubernetes](https://img.shields.io/badge/kubernetes-1.30%2B-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io)

**[Quickstart](#quickstart)** · **[Architecture](#architecture)** · **[Documentation](https://axem-solutions.github.io/shaide/)** · **[Contributing](CONTRIBUTING.md)** · **[Join the Community](https://discord.gg/n7nUneE2Ga)**

</div>

> [!IMPORTANT]
> **shaide is in early access.** The platform runs production workloads today, but this
> repository is young: interfaces, stack configuration and module layout may still change
> between releases. Pin a release rather than tracking `main`, and please open an issue if
> something breaks.

---

## What is shaide?

shaide is a self-hosted AI platform that serves AI models at scale on your
own Kubernetes clusters. The goal is to run many models side by side, each with multiple
replicas, and route traffic across them.

Standing up enterprise AI infrastructure today means assembling a long list of moving
parts - an inference engine, a serving orchestrator, a gateway, a model registry,
storage, observability, and that is only the beginning. Each one has to be chosen,
configured and glued to the next, component by component, then again for every
environment. shaide ships that whole stack as one installable platform.

The entire platform is managed as **infrastructure as code**. Every layer - the internal
registry, the gateway, model serving and the application layer - is defined as a Pulumi
project in this repository, so your AI infrastructure is versioned and reproducible.

And it stays inside your perimeter. shaide is built for organisations whose data cannot
leave their infrastructure: regulated industries, defence, public sector, or anyone who
simply will not send prompts to a third-party API. There is nothing phoning
home and no dependency on a vendor's cloud - including fully **air-gapped**
installations with no internet access at all.

## Why shaide

- **Sovereign by design.** Everything runs in your infrastructure. An internal OCI
  registry mirrors every container image and model weight, so a cluster can operate with
  no egress whatsoever.

- **Installed by one command.** Point the interactive terminal installer at a prepared
  cluster and it installs the entire platform through guided prompts. No Helm chart
  archaeology, no twelve READMEs to follow in order.

- **Distributed multi-model serving.** [vLLM](https://vllm.ai/) as the inference engine,
  [llm-d](https://llm-d.ai/) for multi-instance orchestration. Serve generative and embedding models
  concurrently, each scaled independently across GPU nodes.

- **Built for agent fleets.** Multi-model routing, KV-cache-aware scheduling and
  inference-pool load balancing keep many concurrent agents served from one platform.

- **Runs anywhere Kubernetes runs.** AWS EKS, GCP GKE, Azure AKS and on-prem RKE2 -
  the same platform and the same installer across all of them.

- **OpenAI-compatible API.** Point any OpenAI-compatible SDK, agent framework or tool at
  your endpoint and change nothing but the base URL.

- **Infrastructure as Code.** Every layer is a [Pulumi](https://www.pulumi.com/) Go program, so
  deployments are reviewable, diffable and reproducible.

## Quickstart

### 1. Prepare a cluster

shaide installs onto an **existing** Kubernetes cluster. Check it against the prerequisites:
> **[→ Prerequisites](docs/cluster-requirements/overview.md)**

### 2. Choose your models

The installer ships with everything it deploys. The one input you supply is a **model
manifest** listing the models to publish into the internal registry.

> [!IMPORTANT]
> Supplying `models.yaml` by hand is a temporary step. Model selection moves into the
> installer in the next release, and this file will no longer be required.

```bash
mkdir -p /tmp/manifests
cat > /tmp/manifests/models.yaml <<'YAML'
models:
  - id: "openai/gpt-oss-20b"
    revision: "6cee5e81ee83917806bbde320786a8fb61efebee"
    harbor_project: "ai-models"
    harbor_name: "gpt-oss-20b"
    harbor_tag: "1.0.0"
YAML
```

### 3. Run the installer

Create a directory to hold the installer data and run the installer container:

```bash
STORAGE_PATH=<storage-path>
PULUMI_CONFIG_PASSPHRASE=<choose-a-passphrase>
HF_TOKEN=<your-huggingface-token>

mkdir -p "${STORAGE_PATH}"

docker run --rm -it \
  --network host \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e HF_TOKEN \
  -e MODEL_MANIFEST_PATH=/manifests/models.yaml \
  -v "$HOME/.kube/config:/.kube/config:ro" \
  -v /tmp/manifests/models.yaml:/manifests/models.yaml:ro \
  --mount "type=bind,src=${STORAGE_PATH},dst=/var/shaide-installer" \
  ghcr.io/axem-solutions/shaide/installer:dev
```

> **[→ Installer guide](docs/installation/installer-guide.md)**

### 4. Verify

Once the installer completes, check that the platform is serving:

```bash
curl https://<your-shaide-endpoint>/v1/models
```

### Using the API

shaide exposes an OpenAI-compatible API, so existing clients work unchanged:

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://<your-shaide-endpoint>/v1",
    api_key="<your-api-key>",
)

response = client.chat.completions.create(
    model="<model-name>",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)
```

## Architecture

<div align="center">
  <img alt="shaide architecture" src="docs/assets/architecture.png" width="800">
</div>

shaide is built in layers, each deployed as an independent Pulumi program:

| Layer | What it does |
| --- | --- |
| **Platform services** | The internal OCI registry that holds every image and model weight, plus the shared Istio Gateway and Gateway API layer. |
| **Serving** | Per-model inference stacks: vLLM engine pods orchestrated by llm-d, fronted by an inference gateway that routes and load-balances across replicas. |
| **Application** | The shaide server - the universal API surface, authentication and user management - together with the control panel UI. |
| **Packages** | The interactive installer, the observability stack, and shared libraries used across every layer. |

Deployment proceeds in a fixed order:

```
1. Internal OCI registry       harbor
2. Gateway + Istio             infra/gateway-provider
3. Model serving               app_serving
4. Application layer           app_shaide
5. Optional add-ons            app_mcp, monitoring
```

## The shaide components

shaide is developed across three open-source repositories:

| Repository | Role |
| --- | --- |
| **[shaide](https://github.com/axem-solutions/shaide)** | This repository - the core: infrastructure, model serving and the installer. |
| **[shaide_server](https://github.com/axem-solutions/shaide_server)** | The universal interface to every service the platform provides, plus authentication and user management. |
| **[shaide_control_panel](https://github.com/axem-solutions/shaide_control_panel)** | The web UI for operating the platform. |

## Repository layout

```
├── app_serving/      Per-model LLM serving stacks (vLLM + llm-d)
├── app_shaide/       Application layer: shaide server and control panel
├── app_mcp/          MCP server datasources deployed into the shared gateway
├── harbor/           Internal OCI registry for images and model weights
├── infra/            In-cluster platform services: the shared gateway
├── installer/        Containerized interactive installer
├── monitoring/       Observability stack
└── pkg/              Shared Go module with the Pulumi deployment logic

```

Full technical documentation lives at **[docs](docs)**.

## Contributing

Contributions are welcome. See **[CONTRIBUTING.md](CONTRIBUTING.md)** for development
setup, coding standards and the review process.

## Security

Please do not report security vulnerabilities through public issues. See our
**[security policy](https://github.com/axem-solutions/shaide?tab=security-ov-file)** for
how to disclose them responsibly.

## License

Licensed under the Apache License 2.0. See **[LICENSE](LICENSE)** for details.

shaide
Copyright 2026 axem solutions Kft.

This product includes software developed by third parties. See their respective licensing for 
additional copyright and licensing information.
