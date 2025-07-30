tasks {
  task("compile") {
    description = "Compile source code"
    action = "javac src/**/*.java -d build/classes"
  }

  task("test") {
    description = "Run unit tests"
    action = "java -jar test-runner.jar"
    dependsOn("compile")
  }

  task("package") {
    description = "Package artifact"
    action = "jar cf build/app.jar -C build/classes ."
    dependsOn("test")
  }

  task("deploy") {
    description = "Deploy to server"
    action = "scp build/app.jar user@server:/deployments/"
    dependsOn("package")
  }
}