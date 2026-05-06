# bu1ld

`bu1ld` is an early cross-language build tool prototype.

The first version includes:

- Cobra command layout: `init`, `build`, `test`, `doctor`, `graph`, `clean`
- Task discovery via `tasks` and targeted graph planning via `graph [task...]`
- Multi-process `cmd/cli`, `cmd/daemon`, `cmd/server`, and `cmd/lsp` executables
- A small `build.bu1ld` DSL
- A basic DSL language server with parse diagnostics, semantic diagnostics, and schema completions
- A plugin registry with builtin, local, and global plugin sources
- Builtin `go`, `docker`, and `archive` plugins
- Monorepo workspace package discovery and package-scoped task ids
- Task graph planning with dependency ordering and cycle detection
- A configuration cache for unchanged build scripts and plugin binaries
- Input fingerprints and a local action cache
- Cached output blobs for declared outputs
- A full `arcgolabs/dix` runtime per subcommand
- `arcgolabs/collectionx`, `configx`, `eventx`, and `logx` integration

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
├── integration/
│   └── vscode/
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

git.info version {
  out = "dist/git-info.json"
  include_dirty = true
}

toolchain go {
  version = env("GO_VERSION", "1.26.2")
}

import "tasks/go.bu1ld"
```

Monorepo workspaces can discover package build files from the root build file:

```text
workspace {
  name = "mono"
  packages = ["apps/*", "libs/*"]
}
```

Each package can declare metadata and local tasks in its own `build.bu1ld`:

```text
package {
  name = "apps/api"
  deps = ["libs/core"]
}

task build {
  inputs = ["src/**"]
  outputs = ["dist/**"]
  command = ["go", "build", "./..."]
}
```

Package tasks are exposed as globally unique ids such as `apps/api:build`.
Package dependencies automatically add same-name task dependencies, so
`apps/api:build` depends on `libs/core:build` when both packages define
`build`.

```text
# tasks/go.bu1ld
go.test test {
  packages = ["./..."]
}

go.binary build {
  deps = [test]
  main = "./cmd/cli"
  out = $("dist/" + "bu1ld")
}
```

Builtin Docker and archive rules use native Go implementations instead of
shelling out to `docker`, `zip`, or `tar` commands:

```text
docker.image app_image {
  context = "."
  dockerfile = "Dockerfile"
  tags = ["example/app:dev"]
  build_args = { VERSION = "dev" }
  load = true
}

archive.zip package_zip {
  deps = [build]
  srcs = ["dist/**"]
  out = "dist/package.zip"
}

archive.tar package_tgz {
  deps = [build]
  srcs = ["dist/**"]
  out = "dist/package.tar.gz"
  gzip = true
}
```

`docker.image` builds through the Docker Go SDK and Docker daemon API. The
first implementation supports local image builds, optional single-platform
selection, build args, target stage selection, and pushing tags after build.
Multi-platform buildx exports are intentionally left for a later Docker
BuildKit iteration.

Block names are typed symbols instead of string labels. Expressions are parsed
directly by `plano`, while imports are resolved relative to the file that
declares them and also support doublestar glob patterns such as
`import "tasks/**/*.bu1ld"`.

Custom tasks can use the same built-in functions and expression context:

```text
task package {
  outputs = ["dist/package"]
  run {
    shell("echo package")
  }
}

task archive {
  deps = [build]
  inputs = ["dist/bu1ld"]
  outputs = ["dist/pack.tgz"]
  run {
    exec("tar", "-czf", "dist/pack.tgz", "dist/bu1ld")
  }
}
```

Recent `plano` syntax is available in `.bu1ld` files, including membership
checks, conditional expressions, and filtered loops:

```text
task package_if_needed {
  let formats = ["zip", "tar"]
  outputs = "zip" in formats ? ["dist/package.zip"] : []
}

task selected_test {
  let packages = ["./...", "./cmd/...", "./internal/..."]
  for pkg in packages where pkg != "./..." {
    command = ["go", "test", pkg]
    break
  }
}
```

`shell(...)` actions are parsed as POSIX shell through `mvdan.cc/sh/v3` before
execution. Use `exec(...)` when the task should avoid shell parsing and pass an
argv list directly to the process runner.

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
The current builtins are `go`, `docker`, `archive`, and `git`. The `git.info`
rule uses go-git to write repository metadata such as HEAD, branch, commit,
dirty state, and remotes into a JSON output.
Local plugins are external process plugins resolved under the project
`.bu1ld/plugins` directory by default. Global plugins are resolved under the
user home `.bu1ld/plugins` directory by default. A `path = "./..."` value can be
used for local plugin development. External process plugins implement the public
`pkg/pluginapi` protocol and are launched through HashiCorp `go-plugin`. When
the exact install path is missing, local and global plugin resolution falls back
to `go-plugin` discovery under the corresponding plugin directory.

Installed plugins can include a manifest at
`.bu1ld/plugins/<id>/<version>/plugin.json` or
`~/.bu1ld/plugins/<id>/<version>/plugin.json`:

```json
{
  "id": "org.bu1ld.rust",
  "namespace": "rust",
  "version": "0.1.0",
  "binary": "bu1ld-rust",
  "rules": [{ "name": "binary" }]
}
```

## Usage

```bash
mkdir hello-bu1ld
cd hello-bu1ld
go run ../bu1ld/cmd/cli init
go run ../bu1ld/cmd/cli doctor
go run ../bu1ld/cmd/cli tasks
go run ../bu1ld/cmd/cli build
```

Inside this repository:

```bash
go run ./cmd/cli init --project-dir /tmp/hello-bu1ld
go run ./cmd/cli doctor
go run ./cmd/cli packages
go run ./cmd/cli packages graph
go run ./cmd/cli affected --base main
go run ./cmd/cli build --all :build
go run ./cmd/cli graph
go run ./cmd/cli graph build
go run ./cmd/cli tasks
go run ./cmd/cli test
go run ./cmd/cli build
go run ./cmd/cli clean
go run ./cmd/cli plugins list
go run ./cmd/cli plugins doctor
go run ./cmd/cli plugins lock
go run ./cmd/daemon status
go run ./cmd/server status
go run ./cmd/lsp stdio
```

Runnable examples live under `examples/archive-basic` and
`examples/docker-image`. The archive example is covered by CLI end-to-end tests;
the Docker example requires a local Docker daemon.

`plugins list` prints builtin, declared, and manifest-discovered plugins with
source, namespace, resolved path, rules, and status. `plugins doctor` also
checks the local and global plugin directories and returns a non-zero exit when
a plugin is missing, not executable, has an invalid manifest, or cannot complete
the `go-plugin` handshake.

`plugins lock` writes `bu1ld.lock` with declared plugin source, namespace, id,
version, resolved path, and binary checksum. When `bu1ld.lock` exists,
`plugins doctor` verifies locked plugin paths and checksums.

Project configuration is cached under `.bu1ld/cache/config/project.bin`.
`bu1ld` reuses the evaluated task graph when the root build file, imported
files, import glob expansions, environment variables read through `env(...)`,
and external plugin binaries are unchanged. Pass `--no-cache` to bypass both
the configuration cache and build action cache.

Optional config files are loaded through `configx` from `bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, or their `.bu1ld.*` variants.

## Editor Integrations

The VS Code extension lives under `integration/vscode`. It registers `.bu1ld`
files, provides syntax highlighting and snippets, and starts a bundled language
server over stdio. If no bundled server exists for the current platform, it
falls back to `bu1ld-lsp` from `PATH`; `bu1ld.lsp.path` can override this.

```bash
cd integration/vscode
pnpm install
pnpm run build:server
pnpm run compile
```

Use `pnpm run build:server:all` before packaging a release that should include
all supported platform binaries.

## Test

```bash
go test ./...
```
