import * as fs from "node:fs";

/**
 * Inserts a new CHANGELOG entry for the given version just before the first
 * existing release section (or after an [Unreleased] block if present).
 * Writes a standard header first if the file is missing or empty.
 */
export function insertChangelog(
  filePath: string,
  snippet: string,
  version: string
): void {
  const date = new Date().toISOString().split("T")[0];
  const bare = version.startsWith("v") ? version.slice(1) : version;
  const entry = `## [${bare}] - ${date}\n\n${snippet.trim()}`;

  let content = fs.existsSync(filePath)
    ? fs.readFileSync(filePath, "utf-8")
    : "";

  if (content.trim() === "") {
    fs.writeFileSync(
      filePath,
      `# Changelog\n\nAll notable changes to this project will be documented in this file.\n\nThe format is based on Keep a Changelog, and this project follows Semantic Versioning.\n\n${entry}\n`
    );
    return;
  }

  if (content.includes("## [Unreleased]")) {
    const lines = content.split("\n");
    let nextSectionIdx = -1;
    let inUnreleased = false;
    for (let i = 0; i < lines.length; i++) {
      if (lines[i] === "## [Unreleased]") { inUnreleased = true; continue; }
      if (inUnreleased && /^## /.test(lines[i])) { nextSectionIdx = i; break; }
    }
    if (nextSectionIdx === -1) {
      content = content.trimEnd() + `\n\n${entry}\n`;
    } else {
      lines.splice(nextSectionIdx, 0, "", entry, "");
      content = lines.join("\n");
    }
  } else if (/^## \[/m.test(content)) {
    // Insert before the first versioned section
    content = content.replace(/^(## \[)/m, `${entry}\n\n$1`);
  } else {
    // Append at the end
    content += `\n${entry}\n`;
  }

  fs.writeFileSync(filePath, content);
}
