import { readdirSync, realpathSync } from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { markdown } from "../document.js";
import {
  type Adapter,
  ancestors,
  git,
  type Origin,
  type Source,
  source,
} from "./common.js";

function nestedDirectories(root: string) {
  const directories: string[] = [];
  for (const entry of readdirSync(root, { withFileTypes: true }).sort((a, b) =>
    a.name.localeCompare(b.name),
  )) {
    if (
      !entry.isDirectory() ||
      entry.name.startsWith(".") ||
      entry.name === "node_modules"
    )
      continue;
    const directory = join(root, entry.name);
    directories.push(directory, ...nestedDirectories(directory));
  }
  return directories;
}

function imports(text: string) {
  return markdown
    .parse(text, {})
    .flatMap((token) =>
      token.type === "inline"
        ? (token.children ?? []).flatMap((child) =>
            child.type === "text"
              ? [
                  ...child.content.matchAll(
                    /(?:^|[\s(])@((?:~\/|\.{1,2}\/|\/)?[\w.-][\w./-]*)(?=$|[\s),;])/g,
                  ),
                ].map((match) => match[1])
              : [],
          )
        : [],
    );
}

export const claude: Adapter = {
  name: "claude",
  discover({ cwd, root, home }) {
    const config = process.env.CLAUDE_CONFIG_DIR || join(home, ".claude");
    const result: Source[] = [];
    const seen = new Set<string>();

    function add(path: string, origin: Origin, via?: string, depth = 0) {
      const file = source(path, origin);
      if (!file) return;
      const key = realpathSync(path);
      if (seen.has(key)) return;
      seen.add(key);
      result.push({ ...file, via });
      if (depth >= 5 || origin === "Auto memory") return;
      for (const target of imports(file.text)) {
        const imported = target.startsWith("~/")
          ? join(home, target.slice(2))
          : resolve(dirname(path), target);
        add(imported, origin, path, depth + 1);
      }
    }

    add(join(config, "CLAUDE.md"), "Global");
    function addProject(directory: string, origin: Origin) {
      add(join(directory, "CLAUDE.md"), origin);
      add(join(directory, ".claude", "CLAUDE.md"), origin);
    }
    for (const directory of ancestors(root, cwd))
      addProject(directory, "Project");

    const commonGit = git(cwd, "rev-parse", "--git-common-dir");
    const commonRoot =
      commonGit && isAbsolute(commonGit) ? dirname(commonGit) : root;
    const slug = commonRoot.replace(/[^a-zA-Z0-9-]/g, "-");
    add(join(config, "projects", slug, "memory", "MEMORY.md"), "Auto memory");

    for (const directory of nestedDirectories(cwd))
      addProject(directory, "Nested");
    return result;
  },
};
