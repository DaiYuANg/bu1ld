# Go Plugin

The first-party Go plugin is an external process plugin under `plugins/go`.
It is intentionally separate from the bu1ld binary so it can evolve as a normal
plugin and participate in the same registry, install, lock, and process protocol
model as plugins written in other languages.

The plugin does not replace the Go toolchain. It adapts `go build`, `go test`,
`go generate`, and GoReleaser into bu1ld's task graph, inputs/outputs, cache
model, and remote cache integration. Go package loading, module behavior, and
compiler semantics stay owned by upstream Go tools.

## Import Mode

Projects can ask the plugin to import conventional Go toolchain tasks from an
existing `go.mod` or `go.work` project:

```text
go {
  packages = ["./..."]
}
```

This registers:

- `go.generate`: runs `go generate`.
- `go.test`: depends on `go.generate` and runs `go test`.
- `go.build`: depends on `go.test` and runs `go build`.

Set `main` and `out` to make `go.build` produce a binary through the same path
as `go.binary`:

```text
go {
  main = "./cmd/app"
  out = "dist/app"
}
```

Fields:

- `import_tasks`: defaults to `true`.
- `task_prefix`: defaults to `go.`.
- `generate`, `test`, `build`: toggle imported tasks.
- `packages`: defaults to `["./..."]`.
- `main`, `out`: optional binary output mapping for `go.build`.
- `generate_out`: defaults to `build/generated/go`.
- `inputs`, `srcs`, `cacheprog`.

## Build And Install

For local development:

```bash
go build -C plugins/go -o ../../.bu1ld/plugins/org.bu1ld.go/0.1.4/bu1ld-go-plugin ./cmd/bu1ld-go-plugin
```

On Windows, use `bu1ld-go-plugin.exe` in both the output path and manifest
binary field. The manifest is `plugins/go/plugin.toml` and can be copied beside
the plugin binary.

Projects opt in with a normal plugin declaration:

```text
plugin go {
  source = local
  id = "org.bu1ld.go"
  version = "0.1.4"
}
```

## Rules

The plugin namespace is `go`.

### `go.binary`

Builds a Go main package.

```text
go.binary build {
  deps = [test]
  main = "./cmd/bu1ld"
  out = "dist/app"
}
```

Fields:

- `main`: required package path.
- `out`: required output binary path.
- `deps`, `inputs`, `srcs`: task graph and cache inputs.
- `cacheprog`: optional `GOCACHEPROG` override.

### `go.test`

Runs `go test`.

```text
go.test test {
  packages = ["./..."]
}
```

Fields:

- `packages`: defaults to `["./..."]`.
- `deps`, `inputs`, `srcs`.
- `cacheprog`: optional `GOCACHEPROG` override.

### `go.generate`

Runs `go generate` and gives generators a stable output directory.

```text
go.generate generate {
  out = "build/generated/go"
}
```

Fields:

- `packages`: defaults to `["./..."]`.
- `args`: extra arguments before packages.
- `run`: passed to `go generate -run`.
- `skip`: passed to `go generate -skip`.
- `out`: defaults to `build/generated/go`.
- `outputs`: defaults to `<out>/**`.
- `deps`, `inputs`, `srcs`.
- `cacheprog`: optional `GOCACHEPROG` override.

Before execution the plugin creates `out` and injects:

- `BU1LD_GO_GENERATE_OUT`: absolute output directory.
- `BU1LD_GO_GENERATE_REL_OUT`: project-relative output directory.

These variables are intended for `//go:generate` directives.

### `go.release`

Runs GoReleaser through the plugin.

```text
go.release snapshot {
  deps = [test]
  mode = "snapshot"
  config = ".goreleaser.yaml"
}
```

Fields:

- `mode`: `snapshot`, `release`, `check`, or a custom first argument.
- `config`: defaults to `.goreleaser.yaml`.
- `args`: full GoReleaser argument override.
- `command`: explicit GoReleaser binary.
- `module`: fallback module, default `github.com/goreleaser/goreleaser/v2`.
- `version`: fallback module version, default `v2.15.4`.
- `prefer_local`: defaults to `true`; use local `goreleaser` from `PATH` first.
- `deps`, `inputs`, `outputs`.

When no local binary is selected, the plugin runs:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 ...
```

## Go Cacheprog

The plugin can inject `GOCACHEPROG` for `go.binary`, `go.test`, and
`go.generate`.

Precedence:

1. Rule field `cacheprog`.
2. `BU1LD_GO__CACHEPROG` or `BU1LD_GO_CACHEPROG`.
3. Derived command from `BU1LD_GO__REMOTE_CACHE_URL`,
   `BU1LD_GO_REMOTE_CACHE_URL`, `BU1LD_REMOTE_CACHE__URL`, or
   `BU1LD_REMOTE_CACHE_URL`.

With a remote cache URL:

```dotenv
BU1LD_REMOTE_CACHE__URL=http://192.168.1.10:19876
BU1LD_REMOTE_CACHE__PULL=true
BU1LD_REMOTE_CACHE__PUSH=true
```

the plugin injects:

```text
GOCACHEPROG=bu1ld-go-plugin cacheprog --remote-cache-url http://192.168.1.10:19876
```

In installed plugins the command uses the resolved `bu1ld-go-plugin` executable
path. The `cacheprog` subcommand speaks Go's stdin/stdout cacheprog protocol
locally and stores Go action/output records in the bu1ld coordinator.

See [Remote Cache](remote-cache.md) for the coordinator side.

## Standalone Release

The Go plugin has its own GoReleaser configuration:

```bash
cd plugins/go
goreleaser release --snapshot --clean --skip=publish
```

The root release also packages the Go plugin with `plugins/go/plugin.toml`
beside the binary.
