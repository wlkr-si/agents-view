import { claude } from "./adapters/claude.js";
import { codex } from "./adapters/codex.js";
import { context } from "./adapters/common.js";

export const adapters = [claude, codex];

export function discover(cwd: string, agent?: string) {
  const selected =
    agent === undefined
      ? adapters
      : adapters.filter((adapter) => adapter.name === agent);
  if (!selected.length)
    throw new Error("Choose --agent claude or --agent codex.");
  const project = context(cwd);
  return selected.map((adapter) => ({
    agent: adapter.name,
    files: adapter.discover(project),
  }));
}
