# Releasing ai_platform

Releases are handled by the AI agent. The release script (`scripts/release.ts`) automates everything.

## Standard release

From within the products working directory, ask the agent:

```
/release ai_platform vX.Y.Z
```

or: "release ai_platform vX.Y.Z"

The agent will:
1. Read the git log since the last tag and draft a CHANGELOG entry
2. Ask you to confirm the CHANGELOG content before proceeding
3. Create a `release/vX.Y.Z` branch, update `CHANGELOG.md`, commit, push, and open a PR titled `RELEASE vX.Y.Z`
4. Print the Jira Release notes for you to paste manually into the `ai platform vX.Y.Z` release

**Merge the PR.** CI (`release.yml`) will automatically create the `vX.Y.Z` tag and the GitHub Release.

## Running the script directly

```bash
# Create release branch + PR
npx tsx scripts/release.ts v1.2.0 /path/to/changelog-snippet.md

# Dry-run (prints all actions without executing git push)
npx tsx scripts/release.ts v1.2.0 --dry-run
```

## What happens automatically

| Step | How |
|---|---|
| CHANGELOG entry | AI-generated from conventional commits, confirmed before proceeding |
| Release branch + PR | `scripts/release.ts` |
| Git tag | CI `release.yml` triggered by PR merge |
| GitHub Release | CI `release.yml` |
| Jira Release notes | AI agent generates content; you paste it into `ai platform vX.Y.Z` manually |
