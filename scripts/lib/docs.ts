import * as fs from "node:fs";
import * as path from "node:path";

const INSTALLER_IMAGE = /(ghcr\.io\/axem-solutions\/shaide\/installer:)(latest|v\d+\.\d+\.\d+)/g;

function markdownFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return markdownFiles(full);
    return entry.name.endsWith(".md") ? [full] : [];
  });
}

/**
 * Pins every installer image reference in README.md and docs/ to the given
 * release version, so each published docs version installs its own image.
 * Returns the files that changed.
 */
export function updateDocsVersion(version: string): string[] {
  const changed: string[] = [];
  for (const file of ["README.md", ...markdownFiles("docs")]) {
    const content = fs.readFileSync(file, "utf-8");
    const updated = content.replace(INSTALLER_IMAGE, `$1${version}`);
    if (updated !== content) {
      fs.writeFileSync(file, updated);
      changed.push(file);
    }
  }
  return changed;
}
