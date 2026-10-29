# PR validation

`validation.yml` runs on every PR to `main` and on manual dispatch, with read-only
repository permissions. It tests all packages in `pkg`, `app_shaide` and
`app_mcp`, checks their formatting, runs blocking `go vet ./...` in each module, and
builds docs with `mkdocs build --strict` into `$RUNNER_TEMP/docs-site`. It does
not publish anything.

The recursive `./...` pattern includes `pkg/kube` and future packages within
each matrix module. Vet runs with its default analyzers and propagates every
nonzero exit code. PR and manual validation use the same commands.

From each Go module root, run `go test -count=1 ./...` and the formatting command
in the workflow, followed by `go vet ./...`.
From the repository root, install `docs-requirements.txt` in a virtualenv, then
run `python -m unittest discover -s .github/scripts -p 'test_*.py' -v` and
`mkdocs build --strict --site-dir "$RUNNER_TEMP/docs-site"` (set `RUNNER_TEMP` to
a local temporary directory). Contract tests check the workflow's validation
boundaries and execute its vet shell command with a controlled Go executable
in every matrix module, checking full-package arguments and propagation of
diagnostic and command failures. They do not replace native Go checks or
verification of the published GitHub Actions run.
