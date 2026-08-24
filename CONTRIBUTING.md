# Contributing to the shaide AI Platform

Contributions of code, documentation, bug reports, deployment improvements,
and feature ideas are welcome.

By participating, you agree to follow the axem
[Code of Conduct](https://github.com/axem-solutions/.github/blob/main/CODE_OF_CONDUCT.md).

## Before you start

- Read the documentation to understand the architecture, deployment, and operational behavior.
- Read and follow the
  [policy for AI-assisted contributions](.github/AI_POLICY.md). Disclose any AI
  assistance as described in the policy.
- Search the [existing issues](https://github.com/axem-solutions/shaide_ai_platform/issues)
  before opening a new one.
- Do not open a public issue for a suspected security vulnerability. Report it
  privately to [info@axem.dev](mailto:info@axem.dev).

Questions and proposals are also welcome in
[Discussions](https://github.com/axem-solutions/shaide_ai_platform/discussions)
or on [Discord](https://discord.com/invite/Nv6hSzXruK).

## Repository layout

This repository contains several independently testable Go modules and Pulumi
projects:

- `app_serving`: model serving, gateways, and scheduling;
- `app_shaide`: the central router and public application entry point;
- `app_mcp`: MCP gateway deployment;
- `installer`: central installer;
- `monitoring`: the monitoring stack;
- `infra`: cloud, on-premises, registry, and gateway infrastructure
- `pkg`: shared infrastructure-as-code components.

## Development setup

Fork the repository and clone it with its submodules:

```bash
git clone https://github.com/<your-user>/shaide_ai_platform.git
cd shaide_ai_platform
git switch -c <issue-number>/short-description
```

Tooling:

- install Go
- install Pulumi for infrastructure and application deployment projects
- install Docker, kubectl, Helm, and the relevant cloud CLI only when the
  affected workflow requires them

Pulumi backends, credentials, Kubernetes contexts, cloud projects, and stack
configuration are environment-specific.

## Making changes

- Keep changes focused within the relevant component or shared package.
- Add or update tests for changed behavior.
- Run `gofmt` on changed Go files and keep each module's dependencies tidy.
- Update the relevant README or architecture documentation when deployment,
  configuration, topology, or operational behavior changes.
- Do not commit generated Pulumi state, local kubeconfigs, downloaded chart
  archives, model artifacts, credentials, tokens, or plaintext secrets. Store
  Pulumi secrets with `pulumi config set --secret`.

Use [Conventional Commits](https://www.conventionalcommits.org/) and include a
scope when it makes the affected component clearer:

```text
feat(installer): validate available model storage
fix(app_serving): preserve node placement settings
docs(monitoring): document dashboard deployment
```

## Developer Certificate of Origin

Every commit must carry a `Signed-off-by` trailer certifying that you wrote the
contribution, or otherwise have the right to submit it under this repository's
license, as described by the
[Developer Certificate of Origin](https://developercertificate.org/) (DCO).

With `user.name` and `user.email` configured in git, sign off automatically:

```bash
git commit -s -m "fix(app_serving): preserve node placement settings"
```

The commit's `Author` and `Signed-off-by` identities must match. If a commit is
missing its sign-off, add it before opening the pull request:

```bash
git commit --amend -s
```

## Validate your change

At minimum, format the changed Go files and run the tests from every affected Go
module:

```bash
gofmt -w path/to/changed_file.go
cd path/to/affected-module
go test ./...
```

If you change `pkg`, test `pkg` and each consuming module affected by the
change. This is a multi-module repository, so running `go test ./...` once from
the repository root does not cover all components.

## Open a pull request

Push your branch and open a pull request against `main`, or the integration
branch the related issue belongs to. Complete the pull request template and:

- explain what changed, why, and which components or environments are affected
- list the tests you ran
- include documentation updates in the same pull request

Keep the pull request reviewable, respond to feedback, and ensure all required
checks pass. A maintainer will merge the pull request after approval.
