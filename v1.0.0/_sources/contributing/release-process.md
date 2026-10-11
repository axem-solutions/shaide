---
title: "Release process"
description: "How releases are cut and published."
weight: 40
---

# Release process

Releases are handled by the AI agent. The release script (`scripts/release.ts`) automates everything.

## Standard release

From the repository root, ask the agent:

```
/release vX.Y.Z
```

or: "release shaide vX.Y.Z"

The agent will:
1. Read the git log since the last tag (the source migration commit before the first release) and draft a CHANGELOG entry
2. Review the docs for version references and ask you to confirm the CHANGELOG content and the documentation changes before proceeding
3. Create a `release/vX.Y.Z` branch, update `CHANGELOG.md`, pin the installer image in `README.md` and `docs/` to `vX.Y.Z`, commit, push, open a PR titled `RELEASE vX.Y.Z`, and verify that every installer image reference in the docs points at `vX.Y.Z`
4. Print the Jira Release notes for you to paste manually into the `shaide AI Platform vX.Y.Z` release

**Merge the PR.** CI (`release.yml`) will automatically create the `vX.Y.Z` tag and the GitHub Release, publish the installer image and publish the documentation built from the tag to https://axem-solutions.github.io/shaide/.

## Running the script directly

```bash
# Create release branch + PR
npx tsx scripts/release.ts v1.2.0 /path/to/changelog-snippet.md

# Dry-run (prints the git and gh commands without running them)
npx tsx scripts/release.ts v1.2.0 --dry-run
```

## What happens automatically

| Step | How |
|---|---|
| CHANGELOG entry | AI-generated from conventional commits, confirmed before proceeding |
| Release branch + PR | `scripts/release.ts` |
| Installer image tag in the docs | `scripts/release.ts` |
| Git tag | CI `release.yml` triggered by PR merge |
| GitHub Release | CI `release.yml` |
| Installer image `vX.Y.Z` and `latest` | CI `release.yml` |
| Documentation site | CI `release.yml` dispatches `docs.yml` with the tag |
| Jira Release notes | AI agent generates content; you paste it into `shaide AI Platform vX.Y.Z` manually |

## Publishing the documentation manually

The docs site is versioned with [mike](https://github.com/jimporter/mike): every release is
published to its own folder (`/shaide/vX.Y.Z/`), `latest` points at the newest release, and
the header has a version selector. It is only published on release. To republish a version,
for example after a docs fix:

```bash
# Rebuild the latest release from its tag
gh workflow run docs.yml

# Rebuild a version from another ref
gh workflow run docs.yml -f version=vX.Y.Z -f ref=main
```

To pin the installer image in the docs without cutting a release, run
`npx tsx scripts/docs-version.ts vX.Y.Z`.
