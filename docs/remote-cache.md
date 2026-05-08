# Remote Cache

bu1ld has a local action cache and an optional remote coordinator. The remote
cache is designed for LAN and private network setups first: a coordinator
serves action records, output blobs, and Go build-cache objects over HTTP, while
clients decide independently whether to pull or push.

## Local Cache

The local cache stores:

- Action records: task inputs, action parameters, and output references.
- Blobs: content-addressed task outputs.
- Config cache: evaluated project configuration.
- Go cache entries: Go `ActionID` to `OutputID` metadata for `GOCACHEPROG`.

The default cache directory is `.bu1ld/cache`.

## Coordinator

Start the coordinator:

```bash
go run ./cmd/server coordinator --listen 127.0.0.1:19876
```

or through config:

```dotenv
BU1LD_SERVER__COORDINATOR__LISTEN_ADDR=0.0.0.0:19876
```

The coordinator is implemented with `arcgolabs/httpx` and serves the same cache
store used locally by the CLI.

## Action Cache API

Action records:

```text
GET  /v1/actions/{key}
HEAD /v1/actions/{key}
PUT  /v1/actions/{key}
```

Output blobs:

```text
GET  /v1/blobs/{digest}
HEAD /v1/blobs/{digest}
PUT  /v1/blobs/{digest}
```

Blob digests are content hashes. The server validates uploaded blobs and
rejects action records that reference missing blobs.

## Go Cache API

Go cache action records:

```text
GET  /v1/go/cache/actions/{actionID}
HEAD /v1/go/cache/actions/{actionID}
PUT  /v1/go/cache/actions/{actionID}
```

Go cache outputs:

```text
GET  /v1/go/cache/outputs/{outputID}
HEAD /v1/go/cache/outputs/{outputID}
PUT  /v1/go/cache/outputs/{outputID}
```

`actionID` and `outputID` are the 64-character hex forms of Go's cacheprog
`ActionID` and `OutputID`. Output bytes are stored in the same
content-addressed blob store as bu1ld task outputs.

## Client Configuration

Remote pulls are enabled when a remote cache URL is configured. Remote pushes
are opt-in.

```bash
go run ./cmd/cli build --remote-cache-url http://127.0.0.1:19876 --remote-cache-push
go run ./cmd/cli build --remote-cache-url http://127.0.0.1:19876
```

Equivalent dotenv setup:

```dotenv
BU1LD_REMOTE_CACHE__URL=http://192.168.1.10:19876
BU1LD_REMOTE_CACHE__PULL=true
BU1LD_REMOTE_CACHE__PUSH=false
```

Use `env` in `bu1ld.toml` to select environment-specific dotenv files:

```toml
env = "lan"
```

With `env = "lan"`, bu1ld loads `.env.lan.local`, `.env.lan`, `.env.local`,
and `.env` from the project directory. Normal environment variables override
dotenv values.

## Go Cacheprog Adapter

`bu1ld-go-plugin cacheprog` implements Go's stdin/stdout cacheprog protocol. It
keeps a small local disk cache and optionally pulls/pushes Go cache entries
through the coordinator.

The Go plugin can derive `GOCACHEPROG` automatically from
`BU1LD_REMOTE_CACHE__URL`, or users can override it with:

- `BU1LD_GO__CACHEPROG`
- `BU1LD_GO_CACHEPROG`
- a rule-level `cacheprog = "..."`

The adapter supports Go cacheprog `get`, `put`, and `close`. On remote pull it
fetches the action record, downloads the output, verifies the output digest, and
writes the output to local disk before returning a `DiskPath` to the Go
toolchain. On push it validates the output digest, writes local disk state, then
uploads output and action metadata when remote push is enabled.

The cacheprog adapter is intentionally shipped as a subcommand of the Go plugin
binary so installing `bu1ld-go-plugin` is enough for both JSON-RPC plugin
execution and Go build-cache integration.

## Current Boundary

The coordinator is a cache server, not a distributed scheduler. Distributed
build execution can be layered on top later by adding worker registration and
task assignment while preserving the same cache object model.
