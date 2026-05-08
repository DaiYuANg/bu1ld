# java-junit-project

Java plugin example with a `test` source set and JUnit Jupiter tests.

Prepare the local Java plugin from the repository root:

```powershell
.\plugins\java\gradlew.bat -p plugins/java assemble
```

Then build and test the example:

```powershell
go run ./cmd/cli --project-dir examples/java-junit-project build
```

The build writes:

- `build/libs/hello-java-junit.jar`
- `build/libs/hello-java-junit-sources.jar`
- `build/libs/hello-java-junit-javadoc.jar`
- `build/test-results/test/summary.txt`
- `bu1ld-java.lock`
