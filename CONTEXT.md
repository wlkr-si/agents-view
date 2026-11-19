# Context

The words Agents View uses. Use them in code, docs, commands and the Editor, and no others for these ideas.

| Term | Meaning | Not |
| --- | --- | --- |
| **Agents View** | The product: the CLI and the Editor together. | agents view, the view |
| **the Editor** | The browser screen that `agents-view` opens. | the view, local editor, viewer |
| **AGENTS.md tab** | The Editor's tab for the project's `AGENTS.md`, the only file it edits. | Project tab |
| **section** | A top-level `## Heading` and its body. Text before the first heading is the **Intro**. | block, card |
| **section type** | What a section holds, inferred from its body: **text**, **rules**, **pages**, **diagram** or **overrides**. | kind |
| **Global** | An instruction file that applies to every project for one person, such as `~/.claude/CLAUDE.md`. | User, home |
| **Override** | The action of replacing one Global line for this project, and the line it writes into `## Overrides`. | Change |
| **replacement** | The project's text in an Override, after `→ here:`. | new text |
| **STALE** | An Override whose quoted Global text no longer appears in any Global file. | broken, orphaned |
| **Sources** | The instruction files an agent reads for this project, from every origin. | inventory, context files |
| **origin** | Where a Source comes from: **Global**, **Project**, **Auto memory** or **Nested**. | scope, level |
| **load order** | The order an agent reads its Sources in. | discovery order |
| **Nested** | A Source in a subfolder that an agent reads only when work touches that folder (on demand). | lazy |
| **diagram text** | The Mermaid text inside a diagram section; agents read it, the Editor draws it. | diagram source |
| **budget** | The token limit for `AGENTS.md`: about 1,000, estimated as characters ÷ 4. | limit, quota |
| **theme** | An Omarchy `colors.toml`, used unchanged. | skin, palette file |
