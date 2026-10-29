"""Validation workflow contract; requires PyYAML (included by MkDocs)."""
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/validation.yml"


def check_contract(workflow):
    assert set(workflow) == {"name", "on", "permissions", "concurrency", "jobs"}
    assert set(workflow["on"]) == {"pull_request", "workflow_dispatch"}
    assert workflow["on"]["pull_request"] == {"branches": ["main"]}
    assert workflow["on"]["workflow_dispatch"] is None
    assert workflow["permissions"] == {"contents": "read"}
    assert workflow["concurrency"] == {
        "group": "validation-${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}",
        "cancel-in-progress": True,
    }
    assert set(workflow["jobs"]) == {"go", "docs"}
    go = workflow["jobs"]["go"]
    assert go["strategy"]["matrix"] == {"module": ["pkg", "app_shaide", "app_mcp"]}
    assert go["strategy"]["fail-fast"] is False
    assert go["defaults"]["run"]["working-directory"] == "${{ matrix.module }}"
    assert "env" not in go
    allowed_actions = {"actions/checkout@v4", "actions/setup-go@v5", "actions/setup-python@v5"}
    commands = []
    for job in workflow["jobs"].values():
        assert not set(job) & {"permissions", "environment", "services", "container", "if", "continue-on-error"}
        for step in job["steps"]:
            assert not set(step) & {"env", "if", "continue-on-error"}
            if "uses" in step:
                assert step["uses"] in allowed_actions
                if step["uses"] == "actions/checkout@v4":
                    expected = {"persist-credentials": False}
                    if job is go:
                        expected["fetch-depth"] = 0
                    assert step["with"] == expected
                elif step["uses"] == "actions/setup-go@v5":
                    assert step["with"] == {
                        "go-version-file": "${{ matrix.module }}/go.mod",
                        "cache-dependency-path": "${{ matrix.module }}/go.sum\npkg/go.sum\n",
                    }
                else:
                    assert step["with"] == {
                        "python-version": "3.12", "cache": "pip",
                        "cache-dependency-path": "docs-requirements.txt",
                    }
            else:
                commands.append(step["run"])
    assert commands == [
        'files=$(gofmt -l .)\nif [ -n "$files" ]; then\n  printf \'%s\\n\' "$files"\n  exit 1\nfi\n',
        'go test -count=1 ./...',
        'go vet ./...',
        'python -m pip install -r docs-requirements.txt',
        'python -m unittest discover -s .github/scripts -p \'test_*.py\' -v',
        'mkdocs build --strict --site-dir "$RUNNER_TEMP/docs-site"',
    ]
    assert "secrets." not in yaml.safe_dump(workflow)
    assert "github.token" not in yaml.safe_dump(workflow)


class WorkflowContract(unittest.TestCase):
    def setUp(self):
        self.workflow = yaml.safe_load(WORKFLOW.read_text())

    def test_validation_contract(self):
        check_contract(self.workflow)

    def test_rejects_unsafe_or_incomplete_mutations(self):
        mutations = [
            lambda w: w["on"].update(push={"branches": ["main"]}),
            lambda w: w["on"]["pull_request"].update(paths=["pkg/iac/knowledgecenter/**"]),
            lambda w: w["permissions"].update(contents="write"),
            lambda w: w["jobs"]["go"].update(permissions={"packages": "write"}),
            lambda w: w["jobs"]["docs"].update(environment="github-pages"),
            lambda w: w["jobs"]["docs"]["steps"][0]["with"].update({"persist-credentials": True}),
            lambda w: w["jobs"]["docs"]["steps"].append({"run": "mike deploy --push latest"}),
            lambda w: w["jobs"]["go"]["strategy"]["matrix"].update(module=["pkg"]),
            lambda w: w["jobs"]["go"]["steps"][3].update(run="go test -vet=off ./..."),
            lambda w: w["jobs"]["go"]["steps"][4].update(run="go vet ./iac/..."),
            lambda w: w["jobs"]["go"]["steps"][4].update(run="go vet -composites=false ./..."),
            lambda w: w["jobs"]["go"]["steps"][4].update(run="go vet ./... || true"),
            lambda w: w["jobs"]["go"]["steps"][4].update({"continue-on-error": True}),
            lambda w: w["jobs"]["go"]["steps"].pop(4),
            lambda w: w["jobs"]["docs"]["steps"][-1].update(run="mkdocs build"),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                changed = copy.deepcopy(self.workflow)
                mutate(changed)
                with self.assertRaises(AssertionError):
                    check_contract(changed)


class FullVetCommandBoundary(unittest.TestCase):
    """Execute the workflow shell command against a controlled Go executable."""

    def setUp(self):
        workflow = yaml.safe_load(WORKFLOW.read_text())
        self.go_job = workflow["jobs"]["go"]
        self.command = next(
            step["run"] for step in self.go_job["steps"]
            if step["name"] == "Vet all module packages"
        )

    def run_vet(self, exit_code):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            bin_dir = root / "bin"
            bin_dir.mkdir()
            go = bin_dir / "go"
            go.write_text(
                "#!/usr/bin/env python3\n"
                "import json, os, sys\n"
                "from pathlib import Path\n"
                "with Path(os.environ['VET_CALLS']).open('a') as log:\n"
                "    log.write(json.dumps({'args': sys.argv[1:], 'cwd': os.getcwd()}) + '\\n')\n"
                "print('controlled vet output', file=sys.stderr)\n"
                "sys.exit(int(os.environ['VET_EXIT']))\n"
            )
            go.chmod(0o755)
            for module in self.go_job["strategy"]["matrix"]["module"]:
                with self.subTest(module=module, exit_code=exit_code):
                    cwd = root / self.go_job["defaults"]["run"]["working-directory"].replace(
                        "${{ matrix.module }}", module
                    )
                    # New nested packages and kube must remain in the recursive pattern.
                    (cwd / "future" / "nested").mkdir(parents=True)
                    (cwd / "kube").mkdir()
                    calls = root / f"{module}-calls.jsonl"
                    env = dict(os.environ, PATH=f"{bin_dir}:{os.environ['PATH']}",
                               VET_CALLS=str(calls), VET_EXIT=str(exit_code))
                    result = subprocess.run(
                        ["bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", self.command],
                        cwd=cwd, env=env, capture_output=True, text=True,
                    )
                    self.assertEqual(result.returncode, exit_code)
                    self.assertEqual(result.stderr, "controlled vet output\n")
                    self.assertEqual([json.loads(line) for line in calls.read_text().splitlines()],
                                     [{"args": ["vet", "./..."], "cwd": str(cwd)}])

    def test_full_vet_runs_in_every_matrix_module(self):
        self.run_vet(0)

    def test_vet_diagnostic_failure_propagates(self):
        self.run_vet(1)

    def test_vet_command_failure_propagates(self):
        self.run_vet(17)


if __name__ == "__main__":
    unittest.main()
