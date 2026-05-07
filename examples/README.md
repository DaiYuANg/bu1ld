# Examples

This directory contains small bu1ld projects that exercise builtin rules,
external process plugins, and monorepo package discovery.

- `archive-basic`: builtin archive rules, imports, local cache.
- `docker-image`: builtin Docker image rule.
- `go-project`: first-party external Go plugin with `go.generate`,
  `go.test`, and `go.binary`.
- `java-project`: first-party external Java plugin compiling with the Java
  Compiler API and writing a jar.
- `multilang-monorepo`: Go and Java packages in one workspace.

The plugin examples point directly at this repository's local plugin manifests.
From the repository root, prepare the external plugins before running them:

```bash
go build -C plugins/go -o bu1ld-go-plugin ./cmd/bu1ld-go-plugin
./plugins/java/gradlew -p plugins/java assemble
```

On Windows:

```powershell
go build -C plugins/go -o bu1ld-go-plugin.exe ./cmd/bu1ld-go-plugin
.\plugins\java\gradlew.bat -p plugins/java assemble
```

Then run an example with:

```bash
go run ./cmd/cli --project-dir examples/go-project build
go run ./cmd/cli --project-dir examples/java-project build
go run ./cmd/cli --project-dir examples/multilang-monorepo build
```
