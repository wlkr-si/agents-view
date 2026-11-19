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

## Editor themes

The Editor uses Omarchy v4 `colors.toml` files. Choose **Theme** in the Editor or run `agents-view theme` to list the available themes. An asterisk marks the selection.

| Command | Result |
| --- | --- |
| `agents-view theme use system` | Follow the system appearance with Norwegian Wood Sunrise for dark and Flexoki Light for light |
| `agents-view theme use <name>` | Select a bundled or installed theme, such as `flexoki-light` |
| `agents-view theme use omarchy` | Follow the current Omarchy theme as it changes |
| `agents-view theme add <git-url>` | Use the system Git to clone the given repository, then install and activate its root `colors.toml` |

To install from disk, put `colors.toml` in `~/.config/agents-view/themes/<name>/`. Use lowercase letters, digits, and hyphens for the name. You can also drop the file onto the Editor to install and activate it. A file named `colors.toml` gets a name based on its contents. The Editor saves the original file and updates open tabs without reloading the page.

Agents View stores the selection as `theme = "<name>"` in `~/.config/agents-view/settings.toml`. Set `XDG_CONFIG_HOME` to an absolute path to move both the settings and themes. Omarchy mode reads `~/.local/state/omarchy/current/theme/colors.toml` and also supports the older config-directory location `~/.config/omarchy/current/theme/colors.toml`. Both locations can be symlinks. If the selected theme becomes unavailable or invalid, the Editor uses the system themes until it returns or you select a different theme.

- [Norwegian Wood Sunrise](https://github.com/ArveLomsland/norwegian-wood-sunrise) by Arve Lomsland. Only its `colors.toml` is bundled. The [upstream MIT license request](https://github.com/ArveLomsland/norwegian-wood-sunrise/issues/2) is open.
- [Flexoki Light](https://github.com/basecamp/omarchy/tree/master/themes/flexoki-light) from Omarchy. Its [MIT notice](themes/flexoki-light/LICENSE) ships with the bundled theme.
