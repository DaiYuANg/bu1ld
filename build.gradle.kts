import org.jetbrains.kotlin.gradle.plugin.KotlinPluginWrapper

plugins {
  alias(libs.plugins.kotlin)
  alias(libs.plugins.git)
  alias(libs.plugins.dotenv)
  alias(libs.plugins.version.check)
  alias(libs.plugins.mkdocs)
  alias(libs.plugins.spotless)
}


group = "org.bu1ld"
version = "1.0-SNAPSHOT"

allprojects {
  repositories {
    mavenLocal()
    mavenCentral()
    google()
  }
}

subprojects {

  apply<KotlinPluginWrapper>()

  tasks.test {
    useJUnitPlatform()
  }
}