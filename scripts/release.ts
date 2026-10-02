/**
 * Release script for shaide.
 *
 * Usage:
 *   npx tsx scripts/release.ts <version> [changelog-file] [pr-body-file] [--dry-run]
 *
 * Creates a release branch, updates CHANGELOG, opens a PR.
 * Merging the PR triggers CI to create the tag and GitHub Release.
 */

import * as fs from "node:fs";
import { exec, currentBranch, isClean, tagExists } from "./lib/git";
import { insertChangelog } from "./lib/changelog";

const args = process.argv.slice(2);
if (args.length < 1 || args[0] === "--help") {
  console.error(
    "Usage: npx tsx scripts/release.ts <version> [changelog-file] [pr-body-file] [--dry-run]"
  );
  process.exit(1);
}

const dryRun = args.includes("--dry-run");
const positional = args.filter((a) => !a.startsWith("--"));
const version = positional[0].startsWith("v")
  ? positional[0]
  : `v${positional[0]}`;
const changelogFile = positional[1] ?? null;
const prBodyFile = positional[2] ?? null;

const snippet =
  changelogFile && fs.existsSync(changelogFile)
    ? fs.readFileSync(changelogFile, "utf-8").trim()
    : "";

console.log(`\nshaide release ${version}${dryRun ? " [dry-run]" : ""}`);
console.log("\nPreparing release branch");

if (!isClean()) {
  console.error("Working tree is not clean. Commit or stash changes first.");
  process.exit(1);
}
if (tagExists(version)) {
  console.error(`Tag ${version} already exists.`);
  process.exit(1);
}
if (currentBranch() !== "main") {
  console.error("Must be on main before creating a release branch.");
  process.exit(1);
}

const branch = `release/${version}`;
exec("git", ["checkout", "-b", branch], dryRun);

if (snippet) {
  if (dryRun) {
    console.log(`  [dry-run] insertChangelog CHANGELOG.md for ${version}`);
  } else {
    insertChangelog("CHANGELOG.md", snippet, version);
    console.log("  Updated CHANGELOG.md");
  }
} else {
  console.warn(
    "  Warning: no changelog snippet provided – CHANGELOG.md not updated."
  );
}

if (snippet) exec("git", ["add", "CHANGELOG.md"], dryRun);
exec("git", ["commit", "-s", "-m", `chore(release): ${version}`], dryRun);
exec("git", ["push", "origin", branch], dryRun);

const ghArgs = ["pr", "create", "--title", `RELEASE ${version}`, "--base", "main"];
if (prBodyFile && fs.existsSync(prBodyFile)) {
  ghArgs.push("--body-file", prBodyFile);
} else {
  const prBody = [`Release ${version}`, "", snippet ? "## Changes" : "", snippet]
    .filter((l) => l !== undefined)
    .join("\n")
    .trim();
  ghArgs.push("--body", prBody);
}
exec("gh", ghArgs, dryRun);

console.log(`\nPR created for ${branch}.`);
console.log("After merging, CI will automatically create the tag and GitHub Release.");
