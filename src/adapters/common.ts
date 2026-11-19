import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname } from "node:path";

export type Origin = "Global" | "Project" | "Nested" | "Auto memory";
export type Source = {
  path: string;
  origin: Origin;
  text: string;
  via?: string;
};
export type Context = { cwd: string; root: string; home: string };
export type Adapter = { name: string; discover(context: Context): Source[] };

export function git(cwd: string, ...args: string[]) {
  try {
    return execFileSync("git", ["-C", cwd, ...args], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return "";
  }
}

export function context(cwd: string): Context {
  return {
    cwd,
    root: git(cwd, "rev-parse", "--show-toplevel") || cwd,
    home: homedir(),
  };
}

export function ancestors(root: string, cwd: string) {
  const result = [cwd];
  while (cwd !== root && dirname(cwd) !== cwd) {
    cwd = dirname(cwd);
    result.unshift(cwd);
  }
  return result;
}

export function source(path: string, origin: Origin): Source | undefined {
  try {
    return { path, origin, text: readFileSync(path, "utf8") };
  } catch (error) {
    if (
      ["ENOENT", "ENOTDIR", "EISDIR"].includes(
        (error as NodeJS.ErrnoException).code ?? "",
      )
    )
      return undefined;
    throw error;
  }
}
