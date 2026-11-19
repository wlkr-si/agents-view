# agents-view v0

An open convention for plain Markdown instruction files. This specification may be freely copied and implemented.

- The first line of `AGENTS.md` is `<!-- agents-view v0 -->`.
- A top-level `## Heading` starts a named section. Names are unique. Text before the first heading is the intro, addressed as `Intro` by tools.
- Infer the section type in order. A Mermaid fence makes a **diagram**. A list of Markdown links makes **pages**. Another bullet list makes **rules**. Other content is **text**.
- Every visual section carries agent-readable text. Mermaid diagram text qualifies. A future drawing type can link to a sibling drawing with a text equivalent immediately below the link.
- Keep the whole file to about 1,000 tokens. Estimate by dividing character count by four and rounding up. Tools warn when the estimate exceeds the budget.
- Put pointers to larger documents in `## Pages`. Each line is `- [Name](relative/path) — read when …`. Use the existing `CONTEXT.md` convention for a glossary. These are pointers for agents to follow when relevant.
- Put project-specific replacements in `## Overrides`. Each line is `- Global: "<exact global text>" → here: <replacement>`. Double quotes and backslashes can be escaped inside the quote. The replacement is the project instruction. A quote absent from every Global source is **STALE**.
- `AGENTS.md` is the only editable source of truth. Editors preserve the file outside the section they change.

Sources and their load order follow these file conventions in v0:

- **Claude Code**. Global `~/.claude/CLAUDE.md`, then project `CLAUDE.md` and `.claude/CLAUDE.md` from root to the working directory. Expand `@path` imports beside their importer, including `AGENTS.md` when imported. Follow up to five levels of imports. Resolve the repository's `~/.claude/projects/<slug>/memory/MEMORY.md` if present. Nested Claude files are listed separately in deterministic order and are activated by work in their directories.
- **Codex**. Start with the user directory (`~/.codex` or `CODEX_HOME`), then walk from the Git root to the current directory. Prefer a nonempty `AGENTS.override.md` to `AGENTS.md` at each level.

These rules model on-disk conventions. Runtime settings, managed and dynamically loaded instruction sources, and product-specific truncation are outside this version.
