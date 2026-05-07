# Plugin Registry

The plugin registry is the discovery and distribution metadata layer for
external bu1ld plugins. It is deliberately separate from artifact storage:
registry files describe plugins, versions, and asset locations, while each asset
URL controls where the installable package is downloaded from.

This keeps the registry portable. A team can host metadata in Git, publish
assets to any HTTP server or object store, use a private file share, or point at
a local directory during development.

## Source Model

The default source is `embedded`, which reads the first-party registry compiled
into the CLI. A project can override it with `plugin_registry` in `bu1ld.toml`,
`plugin_registry.source`, `BU1LD_PLUGIN_REGISTRY`, or
`BU1LD_PLUGIN_REGISTRY__SOURCE`.

```toml
[plugin_registry]
source = "git+ssh://git@example.com/platform/bu1ld-plugins.git?ref=v1&path=registry"
```

Supported source forms:

- `embedded`: built-in first-party metadata.
- `./registry`: local directory containing `plugins.toml`.
- `./registry/plugins.toml`: local registry index file.
- `https://example.com/registry`: HTTP(S) directory URL; `plugins.toml` is
  appended.
- `https://example.com/registry/plugins.toml`: HTTP(S) registry index file.
- `git+https://example.com/platform/bu1ld-plugins.git?ref=main&path=registry`:
  Git-backed metadata.

Git is the preferred external source because it is naturally distributed,
works with public and private repositories, and does not bind bu1ld to GitHub
Release or any other hosting provider.

## Git Metadata

A Git registry source is only a metadata source. The Git repository contains
`plugins.toml` and plugin entry TOML files. It does not have to contain plugin
archives or binaries.

Git sources are cloned into `.bu1ld/registries` with `go-git`, refreshed on
each registry load, and keyed by repository URL plus ref. The `ref` query
parameter can be a branch, tag, or commit. The optional `path` query parameter
selects a subdirectory inside the checkout.

```bash
BU1LD_PLUGIN_REGISTRY='git+https://example.com/platform/bu1ld-plugins.git?ref=main&path=registry' \
  bu1ld plugins search
```

The implementation uses the Go `go-git` library instead of shelling out to a
local `git` executable. This keeps registry loading embeddable and testable
inside the bu1ld process.

## Registry Layout

The index is a TOML file named `plugins.toml`:

```toml
version = 1

[[plugins]]
id = "org.example.rust"
file = "plugins/org.example.rust.toml"
```

Each referenced plugin entry declares the plugin identity, namespace, versions,
and installable assets:

```toml
id = "org.example.rust"
namespace = "rust"
owner = "example"
description = "Rust task rules for bu1ld"
homepage = "https://example.com/bu1ld-rust"
tags = ["rust", "cargo"]

[[versions]]
version = "0.1.0"
bu1ld = ">=0.1.0"

[[versions.assets]]
os = "linux"
arch = "amd64"
format = "tar.gz"
url = "https://downloads.example.com/bu1ld/rust/0.1.0/rust-linux-amd64.tar.gz"
sha256 = "..."

[[versions.assets]]
os = "windows"
arch = "amd64"
format = "zip"
url = "https://downloads.example.com/bu1ld/rust/0.1.0/rust-windows-amd64.zip"
sha256 = "..."
```

Asset URLs are resolved relative to the plugin entry file when they are not
absolute HTTP(S), `file://`, or filesystem paths. This is useful for local
registry development:

```toml
[[versions.assets]]
format = "dir"
url = "../assets/rust"
```

Supported asset formats are `zip`, `tar`, `tar.gz`, and local `dir`.

## Operational Guidance

Use a pinned tag or commit for reproducible registry state:

```toml
[plugin_registry]
source = "git+ssh://git@example.com/platform/bu1ld-plugins.git?ref=v1.0.0&path=registry"
```

Use a branch when the team wants a moving internal catalog:

```toml
[plugin_registry]
source = "git+ssh://git@example.com/platform/bu1ld-plugins.git?ref=main&path=registry"
```

Prefer publishing `sha256` for remote assets. The installer verifies the hash
when it is present, which keeps a Git metadata catalog trustworthy even when
assets are served by another system.
