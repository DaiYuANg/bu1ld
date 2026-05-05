# bu1ld VS Code Extension

VS Code language support for `.bu1ld` files.

## Features

- Registers the `bu1ld` language for `.bu1ld` files.
- Provides TextMate syntax highlighting.
- Starts the bu1ld language server over stdio.
- Adds snippets for common workspace, plugin, task, Docker, archive, and Go rules.
- Adds `bu1ld: Restart LSP`.

## Configuration

By default the extension starts the bundled language server for the current
platform:

```bash
server/<platform>-<arch>/bu1ld-lsp stdio
```

If no bundled server is present, it falls back to `bu1ld-lsp` from `PATH`. Set
`bu1ld.lsp.path` when using a local binary, for example:

```json
{
  "bu1ld.lsp.path": "/path/to/bu1ld-lsp",
  "bu1ld.lsp.args": ["stdio"]
}
```

## Development

```bash
pnpm install
pnpm run build:server
pnpm run compile
```

Use `pnpm run build:server:all` before packaging a release that should include
all supported platform binaries.
