task("compile") {
  dependsOn("generateSources")
  doLast {
    exec("kotlinc src -d out")
  }
}