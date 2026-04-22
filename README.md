# bu1ld

`bu1ld` is an early cross-language build tool prototype.

The first version includes:

- Cobra command layout: `build`, `test`, `graph`, `clean`
- Task discovery via `tasks` and targeted graph planning via `graph [task...]`
- Multi-process `cmd/cli`, `cmd/daemon`, `cmd/server`, and `cmd/lsp` executables
- A small `build.bu1ld` DSL
- A basic DSL language server with parse and semantic diagnostics
- A plugin registry with builtin, local, and global plugin sources
- Task graph planning with dependency ordering and cycle detection
- Input fingerprints and a local action cache
- Cached output blobs for declared outputs
- A full `arcgo/dix` runtime per subcommand
- `arcgo/collectionx`, `configx`, `eventx`, and `logx` integration

## Structure

```text
.
├── cmd/
│   ├── cli/
│   │   ├── main.go
│   │   └── root.go
│   ├── daemon/
│   │   └── main.go
│   ├── lsp/
│   │   └── main.go
│   └── server/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── app.go
│   ├── build/
│   ├── cache/
│   ├── config/
│   ├── dsl/
│   ├── engine/
│   ├── events/
│   ├── graph/
│   ├── lsp/
│   ├── plugin/
│   ├── plugins/
│   └── snapshot/
├── pkg/
│   └── pluginapi/
├── build.bu1ld
├── go.mod
└── README.md
```

## DSL

```text
workspace {
  name = "bu1ld"
  default = build
}

plugin go {
  source = builtin
  id = "builtin.go"
}

toolchain go {
  version = $(env("GO_VERSION", "1.26.2"))
}

go.test test {
  packages = ["./..."]
}

go.binary build {
  deps = [test]
  main = "./cmd/cli"
  out = $("dist/" + "bu1ld")
}
```

Block names are typed symbols instead of string labels. The `$(...)` form is
evaluated by `expr-lang/expr`, while the outer build script syntax is parsed
into bu1ld's own AST.

Plugins can come from three sources:

```text
plugin go {
  source = builtin
  id = "builtin.go"
}

plugin rust {
  source = local
  id = "org.bu1ld.rust"
  version = "0.1.0"
}

plugin java {
  source = global
  id = "org.bu1ld.java"
  version = "0.1.0"
}
```

Builtin plugins are native Go implementations linked into the bu1ld binary.
Local plugins are external process plugins resolved under the project
`.bu1ld/plugins` directory by default. Global plugins are resolved under the
user home `.bu1ld/plugins` directory by default. A `path = "./..."` value can be
used for local plugin development. External process plugins implement the public
`pkg/pluginapi` protocol and are launched through HashiCorp `go-plugin`.

## Usage

```bash
go run ./cmd/cli graph
go run ./cmd/cli graph build
go run ./cmd/cli tasks
go run ./cmd/cli test
go run ./cmd/cli build
go run ./cmd/cli clean
go run ./cmd/daemon status
go run ./cmd/server status
go run ./cmd/lsp stdio
```

Optional config files are loaded through `configx` from `bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, or their `.bu1ld.*` variants.

## Test

```bash
go test ./...
```
