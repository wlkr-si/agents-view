<!-- agents-view v0 -->
# agents-view

One plain AGENTS.md serves every agent, the CLI, and the Editor.

## Rules

- Use the shared parser and writer for all AGENTS.md edits.
- Keep the server on loopback. Pass diagram prompts to the local command as stdin.
- Run go build, go vet, gofmt and go run ./tools/budget. Prove behavior with the real CLI and browser.

## Pages

- [Glossary](CONTEXT.md) — read when naming anything in code, docs or the Editor.
- [Specification](SPEC.md) — read when changing the format or how Sources are found.
- [README](README.md) — read when changing installation or commands.
