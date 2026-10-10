---
disable-model-invocation: true
argument-hint: <version>
---

Release shaide. Usage: `/release <version>`

Run this command from the repository root; it operates on the current git repository. VERSION is required in `vX.Y.Z` or `X.Y.Z` format.

---

## Steps to follow

Treat `$ARGUMENTS` as VERSION.

**1. Validate.**

- VERSION is required. Normalize to `vX.Y.Z`. Reject if not valid semver.
- Confirm the current repo is `shaide`: `git remote get-url origin` should end in `/shaide` or `/shaide.git`. If it does not, stop and warn that you are in the wrong directory.

**2. Pre-flight checks.**

Run these three checks before doing anything else:

```bash
# Must be on main
git rev-parse --abbrev-ref HEAD

# Working tree must be clean
git status --porcelain

# Tag must not already exist
git tag -l <VERSION>
```

Stop immediately with a clear message if any fails:
- Branch is not `main` → "Must be on main to release."
- Status output is non-empty → "Working tree is dirty. Commit or stash all changes first: `<list the dirty files>`"
- Tag output is non-empty → "Tag <VERSION> already exists."

**3. Generate CHANGELOG content.**

Determine the baseline (BASE). It is the last tag; before the first tag exists, it is the `chore: migrate the source` commit that imported the code from the predecessor repo, so the migrated history is not listed:
```bash
BASE="$(git describe --tags --abbrev=0 2>/dev/null || \
        echo d05641a23841b75476faffb7fe1dc3422ec50033)"
```

Get commits since BASE:
```bash
git log "$BASE..HEAD" --pretty=format:"%s" --no-merges
```

Categorize commits by conventional commit type into Keep a Changelog format:
- `feat:` / `feat(<scope>):` → **Added**
- `fix:` / `fix(<scope>):` → **Fixed**
- `chore:`, `refactor:`, `perf:`, `docs:`, `ci:`, `ops:` → **Changed**
- `feat!:` / `fix!:` or `BREAKING CHANGE` footer → **Added** with `**Breaking:**` prefix
- Merge commits and `chore(release):` → omit
- Commits without a conventional prefix (e.g. branch-name-style subjects): inspect the commit body and diff with `git show <hash>` and write a clear human-readable description; categorize as **Fixed** or **Changed** based on content.

Strip any trailing PR references (e.g. `(#123)`) and Jira keys (e.g. `(SHD-123)`) from all changelog entries.

Format:
```
### Added
- <feat commit messages>

### Fixed
- <fix commit messages>

### Changed
- <other commit messages>
```
Omit empty sections. Use today's date for the header (the script adds the `## [X.Y.Z] - date` line).

Write to `/tmp/changelog_shaide.md`.

**4. Build the PR body.**

Jira release name: `shaide AI Platform <VERSION>`

Write `/tmp/release_pr_body_shaide.md` following `.github/PULL_REQUEST_TEMPLATE/release.md`:

```markdown
## Release <VERSION>

**Jira release:** shaide AI Platform <VERSION>

## Changes

<changelog content>
```

**5. Review the documentation version.**

The docs site is versioned per release (see `docs/contributing/release-process.md`), so the docs on the release branch must describe VERSION. `scripts/release.ts` pins every installer image reference in `README.md` and `docs/` to VERSION (`scripts/lib/docs.ts`); you check what it will change and find what it cannot.

List the image references the script will pin:
```bash
git grep -nE 'shaide/installer:(latest|v[0-9]+\.[0-9]+\.[0-9]+)' -- README.md docs
```

Find version-specific text the script does not touch. When BASE is a tag, search for the previous version:
```bash
git grep -nF "<BASE>" -- README.md docs
```
Read each hit and decide whether it should name VERSION instead. Prose that describes the current release (for example "the v1.0.0 installer") should; history, changelog-style notes and versions of third-party tools (Kubernetes, Helm charts, CLIs) should not.

Write down the files the script will pin and any prose edits you propose. Do not edit anything yet: the script requires a clean working tree.

**6. Show the PR body and confirm.**

Print the PR body, the documentation changes from step 5 (the files the script will pin, and each proposed prose edit with file and line), and say: "I'm about to create a release branch and open a PR. Proceed? (type 'yes' to confirm or provide corrections)"

Wait for confirmation. If the user provides corrections, update the temp files and the proposed documentation edits.

**7. Run the release script and verify the documentation.**

```bash
npx tsx scripts/release.ts <VERSION> /tmp/changelog_shaide.md /tmp/release_pr_body_shaide.md
```

The script prints `Updated <file>` for every docs file it pinned. Then verify on the release branch:

```bash
# Every installer image reference must point at VERSION; this must print nothing
git grep -nE 'shaide/installer:' -- README.md docs | grep -v 'shaide/installer:<VERSION>'

# The release commit contains the pinned files next to CHANGELOG.md
git show --stat HEAD
```

If the check prints anything, or the user approved prose edits in step 6, make the edits and fold them into the release commit, so the release stays a single `chore(release): <VERSION>` commit (`publish-dev-image.yml` recognizes a release merge by that commit):

```bash
git commit -a --amend -s --no-edit
git push --force-with-lease origin release/<VERSION>
```

If `mkdocs` is installed (`pip install -r docs-requirements.txt`), run `mkdocs build --strict` and fix any errors the same way.

**8. Generate Jira Release notes for manual entry.**

Derive `<repo>` from the git remote URL. Use BASE from step 3 as `<prev-tag>`; when it is the migration commit, write `none (first release)` as the previous version and use the commit hash in the compare link.

Print the following block for the user to copy into the Jira Release "Release notes" field manually:

```
Goal of release: <one sentence summary derived from the CHANGELOG content for this version>
Previous version: <prev-tag>
Github commit changes link: https://github.com/axem-solutions/<repo>/compare/<prev-tag>...<VERSION>
Release notes link for the given version: https://github.com/axem-solutions/<repo>/releases/tag/<VERSION>
```

**9. Report result.**

Report the PR that was opened and the documentation files it updates. Remind the developer:
- Merge the PR to trigger CI, which will create the tag, publish the release and the installer image, and publish the docs to https://axem-solutions.github.io/shaide/<VERSION>/ (also served as `latest`).
- If the "Publish documentation" run fails, republish with `gh workflow run docs.yml -f version=<VERSION>` after fixing the cause.
- Paste the Jira release notes above into the `shaide AI Platform <VERSION>` release in the shaide Jira project.

---

## Example invocation

```
/release v1.2.0
```
