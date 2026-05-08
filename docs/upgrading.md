# Upgrading

This document records compatibility notes for bu1ld releases. Update it before
cutting a tag whenever a release changes CLI flags, cache protocol behavior,
plugin manifests, registry metadata, or first-party plugin behavior.

## 0.1.x

The first public line is expected to treat the plugin model as the main
extension boundary:

- Process plugins are loaded from TOML manifests.
- Registry metadata can come from embedded, local, HTTP(S), or Git sources.
- Approved registry versions are selected by default; pending and rejected
  versions remain in metadata but are not installed.
- Registry assets may include SHA-256 checksums and Ed25519 detached
  signatures.
- Rejected registry versions are preserved for auditability but are skipped by
  install and update selection. The first public release line marks bad
  first-party plugin releases this way instead of silently removing them.
- Remote cache deployments can configure a bearer token, maximum object size,
  total cache size, and max age through `remote_cache.*` config or
  `BU1LD_REMOTE_CACHE__*` environment variables.

Recommended upgrade process:

1. Update `bu1ld` binaries from the release archive or package.
2. Update first-party plugins with `bu1ld plugins update`.
3. Run `bu1ld plugins doctor` to verify installed plugin manifests and locked
   checksums.
4. If remote cache authentication is enabled, set
   `BU1LD_REMOTE_CACHE__TOKEN` for CLI, server, and Go cacheprog environments.
5. If using a private registry, pin the registry Git source to a tag or commit
   for reproducible builds.
