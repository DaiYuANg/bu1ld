plugins {
    alias(libs.plugins.ksp)
}

group = "org.bu1ld"
version = "1.0-SNAPSHOT"

dependencies {
    implementation(enforcedPlatform(libs.koin.bom))
    ksp(enforcedPlatform(libs.koin.bom))
    implementation(libs.koin.core)
    implementation(libs.koin.annotations)

    implementation(libs.clikt)
    implementation(libs.clikt.markdown)

    ksp(libs.koin.ksp.compiler)
    testImplementation(libs.koin.test)
    testImplementation(libs.koin.test.junit5)
}

