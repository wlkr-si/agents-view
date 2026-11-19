# agents-view

Edit a plain `AGENTS.md` in the Editor, a local browser screen. The AGENTS.md tab shows the file. Sources shows the instruction files each agent reads, in load order, with each Override beside the Global line it replaces. Terms are defined in [CONTEXT.md](CONTEXT.md).

Requires Go 1.24 or later to build. The Editor is embedded in the binary.

```sh
gh repo clone wlkr-si/agents-view
cd agents-view
go install -ldflags="-s -w" ./cmd/agents-view
cd /path/to/project
agents-view
```

Run `agents-view` from the project folder to edit its `AGENTS.md`. The Editor listens on a free loopback port unless you set `AGENTS_VIEW_PORT`. Press Ctrl-C to stop it.

| Command | Result |
| --- | --- |
| `agents-view` | Open the Editor |
| `agents-view --no-open` | Print the local Editor address without opening the browser |
| `sections` | List section types and approximate tokens |
| `read [section]` | Print the file or one section |
| `write <section>` | Read a replacement body from stdin, creating the section if needed |
| `check` | Report invalid format, broken pointers, stale overrides, and budget warnings |
| `sources [--agent claude\|codex\|opencode\|gemini\|cursor\|copilot]` | List Sources and approximate tokens in load order |

Diagram changes go to `codex exec -s workspace-write -`. Set `AGENTS_VIEW_AGENT_COMMAND` to a JSON array of arguments to use a different local command that reads its prompt from stdin. The Editor shows Mermaid diagram text offline.

The Sources tab shows Sources for Claude Code and Codex. The CLI also lists Sources for OpenCode, Gemini CLI, Cursor, and GitHub Copilot. See [SPEC.md](SPEC.md).
