import { join } from "node:path";
import { type Adapter, ancestors, source } from "./common.js";

export const codex: Adapter = {
  name: "codex",
  discover({ cwd, root, home }) {
    const entries = [
      {
        directory: process.env.CODEX_HOME || join(home, ".codex"),
        origin: "Global",
      },
      ...ancestors(root, cwd).map(
        (directory) => ({ directory, origin: "Project" }) as const,
      ),
    ] as const;
    return entries.flatMap(({ directory, origin }) => {
      for (const name of ["AGENTS.override.md", "AGENTS.md"]) {
        const file = source(join(directory, name), origin);
        if (file?.text.trim()) return [file];
      }
      return [];
    });
  },
};
