# bu1ld Documentation

This directory holds design notes and operational guides that are too detailed
for the root README.

## Project Position

bu1ld is a lightweight build runtime with a unified plugin interface. It is
designed to be more structured than a Makefile and lighter than Gradle: core
bu1ld owns the task graph, cache model, remote coordination hooks, plugin
protocol, and artifact flow, while plugins import existing language and
packaging project models into that runtime.

First-party language plugins are therefore integration layers, not attempts to
rewrite complete ecosystems. They should read existing build metadata when it
exists, register those tasks in bu1ld, and reuse mature tools and libraries for
compilation, dependency resolution, packaging, or release automation.

## Documents

- [Architecture](architecture.md): command runtime, configuration loading, DSL
  evaluation, graph planning, engine execution, and cache boundaries.
- [Plugin System](plugin-system.md): builtin and process plugin model,
  manifest format, JSON-RPC methods, task registration, and plugin locks.
- [Plugin Registry](plugin-registry.md): registry source model, Git-backed
  metadata, and plugin distribution TOML schema.
- [Go Plugin](go-plugin.md): first-party Go plugin rules, generated output
  defaults, `GOCACHEPROG` wiring, and embedded GoReleaser orchestration.
- [Java Plugin](java-plugin.md): Java plugin build, JPMS packaging, native
  compiler tasks, task registration, RPC server startup, and logging.
- [Node Plugin](node-plugin.md): Node package script import, TypeScript
  compiler fallback, and container packaging.
- [Remote Cache](remote-cache.md): local action cache, HTTP coordinator, Go
  cacheprog adapter, and dotenv-based LAN configuration.
- [Releases](releases.md): GoReleaser, standalone Go plugin releases, Java
  plugin packaging, and tagged release flow.
- [Upgrading](upgrading.md): release-to-release compatibility and operational
  upgrade notes.
