import { spawnSync } from "node:child_process";

/** Run a command, returning stdout. Exits the process on non-zero exit code. */
function run(cmd: string, args: string[], cwd?: string): string {
  const result = spawnSync(cmd, args, {
    cwd: cwd ?? process.cwd(),
    encoding: "utf-8",
    stdio: ["pipe", "pipe", "pipe"],
  });
  if (result.status !== 0) {
    console.error(`\nCommand failed: ${cmd} ${args.join(" ")}`);
    if (result.stderr) console.error(result.stderr.trim());
    process.exit(1);
  }
  return result.stdout.trim();
}

/** Like run() but only prints the command in dry-run mode. */
export function exec(
  cmd: string,
  args: string[],
  dryRun: boolean,
  cwd?: string
): void {
  if (dryRun) {
    console.log(`  [dry-run] ${cmd} ${args.join(" ")}`);
    return;
  }
  run(cmd, args, cwd);
}

export function currentBranch(): string {
  return run("git", ["rev-parse", "--abbrev-ref", "HEAD"]);
}

export function isClean(): boolean {
  return run("git", ["status", "--porcelain"]).length === 0;
}

export function tagExists(tag: string): boolean {
  return run("git", ["tag", "-l", tag]).length > 0;
}
