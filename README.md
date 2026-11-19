# agents-view

Edit a plain `AGENTS.md` in your local browser. Project shows the core file. Sources shows the instruction files Claude Code and Codex discover, with project overrides alongside the original lines.

Requires Git and Node 20 or later.

```sh
gh repo clone wlkr-si/agents-view
cd agents-view
npm ci
npm link
cd /path/to/project
agents-view view
```

You can also run `node /path/to/agents-view/dist/cli.js` from the project folder. The tool edits `AGENTS.md` in the current directory. Use `view --no-open` to print the local address and open it yourself.

| Command | Result |
| --- | --- |
| `sections` | List section types and approximate tokens |
| `read [section]` | Print the file or one section |
| `write <section>` | Read a replacement body from stdin, creating the section if needed |
| `check` | Report invalid format, broken pointers, stale overrides, and budget warnings |
| `sources [--agent claude\|codex]` | List paths and approximate tokens in discovery order |
| `view` | Open the live local editor |

Diagram changes go to `codex exec -s workspace-write -`. Set `AGENTS_VIEW_AGENT_COMMAND` to a JSON array of arguments to use a different local command that reads its prompt from stdin. Mermaid rendering needs access to jsDelivr.

The Sources view covers the v0 file conventions in [SPEC.md](SPEC.md). It estimates whole-file tokens and labels Claude's on-demand nested files.
