# bu1ld

`bu1ld` is an early cross-language build tool prototype.

The first version includes:

- Cobra command layout: `build`, `test`, `graph`, `clean`
- Multi-process `cmd/cli`, `cmd/daemon`, and `cmd/server` executables
- A small `build.bu1ld` DSL
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
│   └── server/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── app.go
│   ├── cli/
│   │   └── root.go
│   ├── build/
│   ├── cache/
│   ├── dsl/
│   ├── engine/
│   ├── events/
│   ├── graph/
│   ├── snapshot/
│   └── config/
│       └── config.go
├── build.bu1ld
├── go.mod
└── README.md
```

## DSL

```text
task "test" {
  inputs = ["build.bu1ld", "go.mod", "go.sum", "**/*.go"]
  outputs = []
  command = ["go", "test", "./..."]
}
```

## Usage

```bash
go run ./cmd/cli graph
go run ./cmd/cli test
go run ./cmd/cli build
go run ./cmd/cli clean
go run ./cmd/daemon status
go run ./cmd/server status
```

Optional config files are loaded through `configx` from `bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, or their `.bu1ld.*` variants.

## Test

```bash
go test ./...
```
