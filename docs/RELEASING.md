# Releasing

1. Tag `main` with the version and push the tag: `git tag v0.1.0 && git push origin v0.1.0`. Actions builds the six archives and `checksums.txt` and publishes the GitHub release.
2. When the release appears, run `go run ./tools/release v0.1.0 --publish` from a clean checkout of the tag. It updates `wlkr-si/homebrew-tap` and publishes the npm package. If npm is signed out, it prints the command to run after signing in.

## Dry run

`go run ./tools/release v0.0.0-dryrun` builds the archives, `checksums.txt`, the Homebrew formula and the npm package under `dist/` without publishing anything. Pack the npm package with `npm pack ./dist/npm/0.0.0-dryrun`.

To try the installers against a local build, serve `dist/` with `python3 -m http.server --bind 127.0.0.1 --directory dist 8000` and set `AGENTS_VIEW_DOWNLOAD_BASE_URL=http://127.0.0.1:8000` for `install.sh` or the npm launcher. HTTP mirrors are limited to loopback.

## Diagram renderer

`go run ./tools/vendor-mermaid` refreshes the bundled Mermaid 11.12.2 renderer and its [MIT license](../web/mermaid.LICENSE), verified by SHA-256.
