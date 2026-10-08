/**
 * Pins the installer image in the docs to a release version.
 *
 * Usage:
 *   npx tsx scripts/docs-version.ts <version>
 *
 * scripts/release.ts runs this on every release branch; run it directly only
 * to fix up the docs of an existing release.
 */

import { updateDocsVersion } from "./lib/docs";

const arg = process.argv[2];
if (!arg || !/^v?\d+\.\d+\.\d+$/.test(arg)) {
  console.error("Usage: npx tsx scripts/docs-version.ts <version>");
  process.exit(1);
}

const version = arg.startsWith("v") ? arg : `v${arg}`;
const changed = updateDocsVersion(version);
console.log(changed.length ? changed.map((f) => `  Updated ${f}`).join("\n") : "  Docs already at " + version);
