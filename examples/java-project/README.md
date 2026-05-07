# java-project

Small Java project using the first-party external Java plugin. The plugin
compiles sources directly through the Java Compiler API and writes
`build/libs/hello-java.jar`.

From the repository root, prepare the local plugin:

```bash
./plugins/java/gradlew -p plugins/java assemble
```

On Windows:

```powershell
.\plugins\java\gradlew.bat -p plugins/java assemble
```

Then build the project:

```bash
go run ./cmd/cli --project-dir examples/java-project build
```
