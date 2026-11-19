# agents-view

Edit a plain `AGENTS.md` in the Editor, a local browser screen. The AGENTS.md tab shows the file. Sources shows the instruction files each agent reads, in load order, with each Override beside the Global line it replaces. Terms are defined in [CONTEXT.md](CONTEXT.md).

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
| `sources [--agent claude\|codex]` | List Sources and approximate tokens in load order |
| `view` | Open the Editor |

Diagram changes go to `codex exec -s workspace-write -`. Set `AGENTS_VIEW_AGENT_COMMAND` to a JSON array of arguments to use a different local command that reads its prompt from stdin. Mermaid rendering needs access to jsDelivr.

The Sources view covers the v0 file conventions in [SPEC.md](SPEC.md). It estimates whole-file tokens and labels Claude's on-demand nested files.
