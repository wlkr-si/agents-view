# agents-view

Edit a plain `AGENTS.md` in the Editor, a local browser screen. The AGENTS.md tab shows the file. Sources shows the instruction files each agent reads, in load order, with each Override beside the Global line it replaces. Terms are defined in [CONTEXT.md](CONTEXT.md).

## Promise

- The Editor's own files are under 20 KB gzipped. Diagrams load only when one is on screen.
- The Editor answers in under 50 ms after launch.
- One binary under 10 MB, with no runtime to install.
- No network calls on its own: no analytics, no update checks, no downloads. `theme add` fetches only the link you give it.

Every change runs `go run ./tools/budget`, which fails when a number is broken.

## Install

- **Homebrew** on macOS and Linux: `brew install wlkr-si/tap/agents-view`
- **npm**: `npx agents-view` to run it, or `npm install -g agents-view` to install the command. The launcher verifies and caches the binary in the npm package directory on first use.
- **Install script** on macOS and Linux: `curl -fsSL https://raw.githubusercontent.com/wlkr-si/agents-view/main/install.sh | sh`. It verifies the latest release and installs to `~/.local/bin`. Set `AGENTS_VIEW_VERSION` to a release tag to pin it or `AGENTS_VIEW_INSTALL_DIR` to change the destination.
- **[GitHub Releases](https://github.com/wlkr-si/agents-view/releases)**: download the archive for your OS and CPU, verify it against `checksums.txt`, and put the extracted binary on your PATH. macOS and Linux support arm64 and amd64. Windows binaries for both are best-effort.

Run `agents-view --version` to see the installed version.

To build from source, clone the repository and run `go install -ldflags="-s -w" ./cmd/agents-view` with Go 1.24 or later. The Editor is embedded in the binary.

Run `agents-view` from the project folder to edit its `AGENTS.md`. The Editor listens on a free loopback port unless you set `AGENTS_VIEW_PORT`. Press Ctrl-C to stop it.

| Command | Result |
| --- | --- |
| `agents-view` | Open the Editor |
| `agents-view --no-open` | Print the local Editor address without opening the browser |
| `agents-view --version` | Print the version |
| `sections` | List section types and approximate tokens |
| `read [section]` | Print the file or one section |
| `write <section>` | Read a replacement body from stdin, creating the section if needed |
| `check` | Report invalid format, broken pointers, stale overrides, and budget warnings |
| `sources [--agent claude\|codex\|opencode\|gemini\|cursor\|copilot]` | List Sources and approximate tokens in load order |

Place the caret in a diagram section or click its Preview link to draw it in the active theme. The Editor loads the bundled Mermaid renderer on first use and draws offline. To request a change, describe it in Preview. The local command receives the prompt on stdin, and Preview updates when the command saves AGENTS.md. The default command is `codex exec -s workspace-write -`. Set `AGENTS_VIEW_AGENT_COMMAND` to a JSON array of arguments to use another command.

To refresh the bundled renderer, run `go run ./tools/vendor-mermaid` from the repository root. It verifies the pinned Mermaid 11.12.2 bundle and its [MIT license](web/mermaid.LICENSE) by SHA-256 before writing them to `web/`.

The Editor lists local Sources for Claude Code, Codex, OpenCode, Gemini CLI, Cursor, and GitHub Copilot. Use ⌘S or Ctrl+S to save, ⌘P or Ctrl+P to jump to a section or Source, and ⌘J or Ctrl+J to toggle Preview and Problems. See [SPEC.md](SPEC.md) for load order.

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

## Build release assets

Run `go run ./tools/release v0.0.0-dryrun` from the repo root to build the six archives, `checksums.txt`, Homebrew formula and versioned npm package under `dist/`. Pack the npm package with `npm pack ./dist/npm/0.0.0-dryrun`.

For local testing, set `AGENTS_VIEW_DOWNLOAD_BASE_URL=http://127.0.0.1:8000` on the install script or npm launcher while serving `dist/` with `python3 -m http.server --bind 127.0.0.1 --directory dist 8000`. Both installers read the archives and checksum file directly from that URL. HTTP mirrors are limited to loopback.

Pushing a version tag publishes GitHub release assets through Actions. After the release appears, check out the tag in a clean checkout and run `go run ./tools/release <tag> --publish` to update the Homebrew tap and publish the matching npm package. If npm is signed out, the tool prints the exact command to run after signing in.
