# Cross-compiling `dbml-tools`

`dbml-tools` is pure Go (all DB drivers are pure-Go too: `modernc.org/sqlite`,
`lib/pq`, `go-sql-driver/mysql`). Cross-compiling needs nothing beyond the Go
toolchain — `CGO_ENABLED=0` keeps it that way.

## Targets

The build script and Makefile target the six platform/arch combinations the
VSCode extension supports, named with VSCode's `vsce package --target`
convention:

| Target          | GOOS / GOARCH       |
| --------------- | ------------------- |
| `linux-x64`     | `linux` / `amd64`   |
| `linux-arm64`   | `linux` / `arm64`   |
| `darwin-x64`    | `darwin` / `amd64`  |
| `darwin-arm64`  | `darwin` / `arm64`  |
| `win32-x64`     | `windows` / `amd64` |
| `win32-arm64`   | `windows` / `arm64` |

## Quick reference

```sh
make build           # native binary → ./dbml-tools
make test            # run the Go test suite
make cross           # cross-compile every target → dist/<target>/
make cross-archives  # cross + .tar.gz / .zip per target
make verify          # sha256sum -c dist/SHA256SUMS
make sync-vscode     # cross + copy into ../dbml-tools-vscode/server-bin/
make clean
```

The `cross` target invokes `./scripts/build-binaries.sh` directly. You can
also call it manually to build a single target:

```sh
./scripts/build-binaries.sh linux-arm64
./scripts/build-binaries.sh linux-x64 darwin-arm64
./scripts/build-binaries.sh --archives
```

## Output

```
dist/
├── linux-x64/dbml-tools
├── linux-arm64/dbml-tools
├── darwin-x64/dbml-tools
├── darwin-arm64/dbml-tools
├── win32-x64/dbml-tools.exe
├── win32-arm64/dbml-tools.exe
└── SHA256SUMS                 # `sha256sum -c` compatible
```

With `--archives`:

```
dist/
├── dbml-tools-0.1.0-linux-x64.tar.gz
├── dbml-tools-0.1.0-linux-arm64.tar.gz
├── dbml-tools-0.1.0-darwin-x64.tar.gz
├── …
└── SHA256SUMS
```

## Version metadata

Every binary embeds three build-time variables via `-ldflags -X`:

- `main.version` — the contents of `./VERSION` (currently `0.1.0`)
- `main.commit` — `git rev-parse --short HEAD`, suffixed with `-dirty` when
  the working tree has uncommitted changes
- `main.buildDate` — ISO-8601 UTC timestamp at build time

Inspect them at runtime:

```sh
./dist/linux-x64/dbml-tools version
# dbml-tools 0.1.0
#   commit:     bbd28af
#   built:      2026-05-25T15:15:36Z
#   go:         go1.22.2
#   platform:   linux/amd64
```

`--version` and `-v` are accepted as aliases for the `version` subcommand.

## Verifying a downloaded binary

`dbml-tools version --sha256` hashes its own executable on disk and prints
the result. The build script also writes a `SHA256SUMS` file listing the
expected hash of every produced binary.

```sh
# What the binary thinks its hash is
./dbml-tools version --sha256 | tail -1
#   sha256:     cf4a3d8095a69273af1a2e20ec4e62ad7c2a63157886c7cfbf872aea8f4f8c14

# What the SHA256SUMS file says it should be
grep linux-x64 dist/SHA256SUMS
# cf4a3d8095a69273af1a2e20ec4e62ad7c2a63157886c7cfbf872aea8f4f8c14  linux-x64/dbml-tools

# Bulk verify everything in dist/
make verify
# or
(cd dist && sha256sum -c SHA256SUMS)
```

A typical release pipeline ships the per-platform `.tar.gz` / `.zip`
archives **and** the `SHA256SUMS` file (ideally signed) so users can verify
what they downloaded.

## Releasing a new version

1. Edit `./VERSION` (e.g. `0.1.0` → `0.2.0`).
2. Commit the change.
3. Tag: `git tag v0.2.0 && git push --tags`.
4. `make cross-archives`.
5. Upload `dist/dbml-tools-0.2.0-*.tar.gz`, `dist/dbml-tools-0.2.0-*.zip`,
   and `dist/SHA256SUMS` to the GitHub release.
6. Sync into the extension repo and package per-platform vsix files:
   ```sh
   make sync-vscode
   (cd ../dbml-tools-vscode && ./scripts/package-all.sh)
   ```

The version inside each `.vsix` will match `./VERSION` because the extension
manifest's `"version"` and the bundled Go binary's `main.version` are bumped
together (you'll need to also bump the extension's `package.json` `"version"`
to keep the two in lockstep — `package.json` is the version VSCode displays).

## Why no GitHub Actions?

This repo deliberately keeps release tooling local. Everything runs from
your machine with `make cross` — no CI to configure, no permissions to
grant, no secrets to manage. The script and Makefile are the contract.

If you later want CI, the same `./scripts/build-binaries.sh` will run
unchanged in any environment with Go installed.
