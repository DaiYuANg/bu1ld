plugins {
  alias(libs.plugins.ksp)
}

group = "org.bu1ld"
version = "1.0-SNAPSHOT"

dependencies {
  implementation(enforcedPlatform(libs.koin.bom))
  ksp(enforcedPlatform(libs.koin.bom))
  implementation(libs.bundles.koin)

  ksp(libs.koin.ksp.compiler)
  implementation(libs.kotlinx.coroutines.core)

  implementation(libs.bundles.logging)

  implementation(libs.mutiny)
  implementation(libs.mutiny.kotlin)
  implementation(libs.mutiny.vertx.core)
}
