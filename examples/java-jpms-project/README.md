# Java JPMS Project

This example uses the external Java plugin to compile a JPMS module with a
Maven dependency on the module path.

Build it from the repository root after assembling the Java plugin:

```bash
go run ./cmd/cli --project-dir examples/java-jpms-project build
```

The project writes:

- `build/classes/java/main`: compiled module classes.
- `build/libs/hello-java-jpms.jar`: application jar.
- `build/docs/javadoc`: module-aware javadocs.
- `bu1ld-java.lock`: dependency checksum lock file.
