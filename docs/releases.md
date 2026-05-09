# Releases

bu1ld uses GoReleaser for Go binaries, Gradle/jpackage for the Java plugin, and
npm for the TypeScript plugin. The release model keeps first-party plugins
visible as normal external plugins instead of hiding them inside the core CLI.

## Root GoReleaser

The root `.goreleaser.yaml` builds:

- `bu1ld`
- `bu1ld-server`
- `bu1ld-daemon`
- `bu1ld-lsp`
- `bu1ld-go-plugin`

It produces cross-platform archives, checksums, and Linux packages. The Go
plugin package includes `plugins/go/plugin.toml` beside the plugin binary so it
can be installed as `org.bu1ld.go`.

Local snapshot:

```bash
goreleaser release --snapshot --clean --skip=publish
```

## Go Plugin Standalone Release

The Go plugin also has an independent config at `plugins/go/.goreleaser.yaml`.
Use it when publishing or testing only the Go plugin:

```bash
cd plugins/go
goreleaser release --snapshot --clean --skip=publish
```

The plugin's `go.release` rule can also invoke GoReleaser from a bu1ld project.
It prefers a local `goreleaser` binary and falls back to:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 ...
```

See [Go Plugin](go-plugin.md) for rule fields.

## Java Plugin Packaging

The Java plugin is packaged by Gradle:

```bash
./plugins/java/gradlew -p plugins/java installBu1ldPlugin
```

This builds a JPMS jpackage app image, writes `plugin.toml`, and installs it
under `.bu1ld/plugins/org.bu1ld.java/0.1.3/`. The Java plugin does not currently
publish MSI, DMG, DEB, or RPM installers; the app image is the plugin artifact.

See [Java Plugin](java-plugin.md) for the packaging model.

## GitHub Release Flow

Tagged releases are handled by `.github/workflows/release.yml`.

```bash
git tag v0.1.3
git push origin v0.1.3
```

The workflow runs:

- root Go tests
- Go plugin tests
- Java plugin Gradle checks
- TypeScript plugin npm checks
- container image build/publish for bu1ld, the Go plugin, the Java plugin, and
  the TypeScript plugin
- root GoReleaser publish
- Java plugin jpackage app-image builds on Linux, Windows, and macOS runners
- release asset checksum and structure verification after all uploads
- release install E2E against the downloaded Linux assets

Pull requests and normal branch pushes are covered by `.github/workflows/ci.yml`
with a Linux/macOS/Windows matrix for Go tests, Go plugin tests, Java plugin
checks, and TypeScript plugin checks. CI also runs a Linux remote-cache E2E that
starts a coordinator, builds the Go plugin cacheprog adapter, and verifies a
second Go build can hit the remote Go cache.

Go plugin release assets are produced by GoReleaser. Java plugin release assets
are produced after GoReleaser creates the GitHub Release: each platform job runs
`plugins/java assemble`, copies the jpackage app image contents and
`plugin.toml` into one staging directory, archives that directory, and uploads
the result to the same release. TypeScript plugin release assets are universal
Node packages containing `plugin.toml`, compiled `dist/` files, and production
`node_modules`.

Java plugin asset names follow:

- `bu1ld-java-plugin_<version>_linux_amd64.tar.gz`
- `bu1ld-java-plugin_<version>_darwin_amd64.tar.gz`
- `bu1ld-java-plugin_<version>_darwin_arm64.tar.gz`
- `bu1ld-java-plugin_<version>_windows_amd64.zip`

TypeScript plugin asset names follow:

- `bu1ld-typescript-plugin_<version>.tar.gz`

Java archive roots contain `plugin.toml` plus the app image contents. The
TypeScript archive root contains `plugin.toml`, `dist/`, package metadata, and
production `node_modules`. This means registry installation and local
`path = ".../plugin.toml"` development both use the same manifest-driven layout.

## Container Images

Tagged releases also publish Linux container images to GitHub Container Registry:

- `ghcr.io/daiyuang/bu1ld:<version>`
- `ghcr.io/daiyuang/bu1ld-go-plugin:<version>`
- `ghcr.io/daiyuang/bu1ld-java-plugin:<version>`
- `ghcr.io/daiyuang/bu1ld-typescript-plugin:<version>`

Each image is also tagged with `<major>.<minor>` and `latest`. The release
workflow lowercases the GitHub owner before building the GHCR image name because
container repository names must be lowercase.

The image build inputs live under `packaging/container/`:

- `bu1ld.Dockerfile` builds the core CLI from source and packages it into an
  Alpine runtime image with Git and SSH tooling.
- `bu1ld-go-plugin.Dockerfile` builds the Go plugin and packages it on top of a
  Go toolchain image with Git and SSH tooling, so plugin actions can run
  `go generate`, `go test`, `go build`, and `go release`.
- `bu1ld-java-plugin.Dockerfile` runs the Java plugin Gradle build in a Linux
  JDK image, then packages the jlink/jpackage app image into a slim Debian
  runtime image.
- `bu1ld-typescript-plugin.Dockerfile` builds the TypeScript plugin with npm
  and packages the compiled JSON-RPC server plus production dependencies on a
  Node runtime image.

The first-party plugin images are compatible with `source = container`:

```text
plugin go {
  source = container
  id = "org.bu1ld.go"
  version = "0.1.3"
  image = "ghcr.io/daiyuang/bu1ld-go-plugin:0.1.3"
}

plugin java {
  source = container
  id = "org.bu1ld.java"
  version = "0.1.3"
  image = "ghcr.io/daiyuang/bu1ld-java-plugin:0.1.3"
}

plugin typescript {
  source = container
  id = "org.bu1ld.typescript"
  version = "0.1.3"
  image = "ghcr.io/daiyuang/bu1ld-typescript-plugin:0.1.3"
}
```

Each Java and TypeScript plugin asset is uploaded with a sibling `.sha256`
file. GoReleaser publishes `checksums.txt` for Go-built assets. The release
workflow downloads the final GitHub Release assets and runs:

```bash
scripts/verify-release-assets.sh dist/release
```

The script verifies `checksums.txt` plus every `*.sha256` file in the release
directory. It also extracts every CLI archive and first-party plugin archive,
checks that `plugin.toml` has the expected id, namespace, and version, and
verifies that the manifest `binary` path exists inside the archive.

After structural verification, the workflow runs:

```bash
scripts/release-e2e.sh dist/release
```

That script builds a local registry from the downloaded release assets, installs
`org.bu1ld.go`, `org.bu1ld.java`, and `org.bu1ld.typescript` with the release
`bu1ld` binary, and runs the Go, Java, TypeScript, and multi-language monorepo
examples. This catches packaging mistakes that checksum verification alone
cannot detect.

## Registry Update

After a release, update the plugin registry metadata with the new plugin version
and asset URLs. The registry should contain metadata only; the actual asset URL
can point at GitHub Release, an object store, an internal HTTP server, or any
other artifact location. The first-party embedded registry already follows this
shape for `org.bu1ld.go`, `org.bu1ld.java`, and `org.bu1ld.typescript`.

See [Plugin Registry](plugin-registry.md).

If a tag produced incomplete or incorrect assets, keep that version in registry
metadata with `status = "rejected"` instead of deleting the history. Approved
version selection skips rejected entries, while registry users can still see why
the version exists.

## Upgrade Notes

Version upgrade guidance lives in [Upgrading](upgrading.md). Before publishing a
tag, add user-facing compatibility notes there, including CLI changes, plugin
manifest changes, cache protocol changes, and registry metadata changes.
