# bu1ld Documentation

This directory holds design notes and operational guides that are too detailed
for the root README.

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
- [Remote Cache](remote-cache.md): local action cache, HTTP coordinator, Go
  cacheprog adapter, and dotenv-based LAN configuration.
- [Releases](releases.md): GoReleaser, standalone Go plugin releases, Java
  plugin packaging, and tagged release flow.
