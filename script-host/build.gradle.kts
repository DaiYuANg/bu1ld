
group = "org.bu1ld"
version = "1.0-SNAPSHOT"


dependencies {
    implementation(libs.kotlin.scripting.common)
    implementation(libs.kotlin.scripting.jvm)
    implementation(libs.kotlin.scripting.jvm.host)
    implementation(projects.scriptDefinition)
}

