pluginManagement {
  repositories {
    mavenLocal()
    mavenCentral()
    gradlePluginPortal()
    google()
  }
}
plugins {
  id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
  id("org.danilopianini.gradle-pre-commit-git-hooks") version "2.0.2"
  id("com.gradle.enterprise") version "3.13.4"
}

enableFeaturePreview("TYPESAFE_PROJECT_ACCESSORS")

buildCache {
  local {
    isEnabled = true
    directory = File(rootProject.projectDir, ".gradle/build-cache")
  }
}
rootProject.name = "bu1ld"
include("cli")
include("script-definition")
include("script-host")
include("daemon")
