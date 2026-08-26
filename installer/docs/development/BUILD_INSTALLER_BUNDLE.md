# Build the installer bundle

The installer bundle is a gzip-compressed tar archive containing all deployment
assets needed by the installer. Stage its contents under
`installer/installer-bundle/bundle/`, then build it with
`installer/installer-bundle/scripts/build-bundle.sh`.

## Bundle contents

The staging directory must have this root layout:

```text
bundle/
|-- checksum.json
|-- deployments/
|-- images/
`-- manifests/
    |-- images.yaml
    `-- models.yaml
```

The runtime directory is named `manifests/` (plural), not `manifest/`.

| Path | What to put there |
| --- | --- |
| `deployments/` | Pulumi projects and stack files, plus all local Helm charts, CRDs, model values, and other files referenced by those projects. Stack files must contain non-secret defaults only. See [stack file configuration](./STACK_FILES_CONFIG.md). |
| `images/` | OCI image archives for entries in `manifests/images.yaml` whose `source` is `archive`. Remote image entries do not need a local archive. |
| `manifests/images.yaml` | The `harbor_upload_images` and `goharbor_images` inventories. Each entry needs `source`, `project`, `name`, and `tag`. |
| `manifests/models.yaml` | The model inventory supplied by the bundle author. Each entry needs `id`, `revision`, `harbor_project`, `harbor_name`, and `harbor_tag`; `dependencies` is optional. |
| `checksum.json` | A generated fingerprint of the other bundle files. Do not maintain it manually. The build script rewrites it and places it first in the archive, as required by the bootstrap refactor. |

For an archived image, the expected filename is derived from its manifest entry:
replace `/` in `name` with `-`, then append `-<tag>.tar`. For example:

```yaml
goharbor_images:
  - source: archive
    project: goharbor
    name: harbor-core
    tag: v2.14.2
```

must have this file:

```text
images/harbor-core-v2.14.2.tar
```

Models are downloaded from Hugging Face during installation and are not stored
under `images/`.

## Build

From the repository root, run:

```bash
installer/installer-bundle/scripts/build-bundle.sh
```

The defaults are:

```text
staging directory: installer/installer-bundle/bundle
output archive:    installer/installer-bundle/bundle.tar.gz
```

Both paths can be overridden:

```bash
installer/installer-bundle/scripts/build-bundle.sh <staging-directory> <output-archive>
```

The script never creates or replaces `manifests/models.yaml`. Write the desired
model inventory into the selected staging directory before running the build;
the file is validated and included exactly as supplied. Model files themselves
are downloaded from Hugging Face during installation.

The script:

1. checks the required directories and manifest files;
2. rejects symlinks and other archive entry types unsupported by the installer;
3. hashes the sorted payload file list and writes `checksum.json`;
4. creates the archive with `checksum.json` as its first entry;
5. verifies the checksum entry before replacing the output archive.

It requires Bash, GNU `tar`, `find`, `sort`, `sha256sum`, `awk`, and `mktemp`.

## Verify

Inspect the completed archive:

```bash
tar -tzf installer/installer-bundle/bundle.tar.gz
tar -xOzf installer/installer-bundle/bundle.tar.gz checksum.json
```

The first command must print `checksum.json` first, followed by the three
payload directories. Rebuilding unchanged staged content produces the same
payload digest in `checksum.json`; changing a payload filename or its contents
changes the digest and makes the installer extract the new bundle.
