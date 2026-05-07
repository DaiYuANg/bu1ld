plugins {
    alias(libs.plugins.jlink)
    alias(libs.plugins.lombok)
    application
}

group = "org.bu1ld.plugins"
version = "0.1.0"

repositories {
    mavenCentral()
}

val javaReleaseVersion = libs.versions.javaRelease.get()
val bu1ldPluginId = "org.bu1ld.java"
val bu1ldPluginNamespace = "java"
val bu1ldPluginVersion = version.toString()
val bu1ldPluginBinary = providers.systemProperty("os.name")
    .map { name ->
        if (name.lowercase().contains("windows")) {
            "bu1ld-java-plugin.exe"
        } else {
            "bu1ld-java-plugin"
        }
    }
    .orElse("bin/bu1ld-java-plugin")

java {
    modularity.inferModulePath.set(true)
}

dependencies {
    implementation(libs.lsp4jJsonrpc)
    implementation(libs.commonsLang3)
    implementation(libs.commonsIo)
    implementation(libs.guava)
    implementation(libs.avajeInject)
    annotationProcessor(libs.avajeInjectGenerator)
    testImplementation(libs.jacksonDatabind)
}

application {
    mainClass.set("org.bu1ld.plugins.java.Bu1ldJavaPlugin")
    applicationName = "bu1ld-java-plugin"
}

tasks.withType<JavaCompile>().configureEach {
    options.encoding = "UTF-8"
    options.release.set(javaReleaseVersion.toInt())
    options.compilerArgs.add("-proc:full")
}

tasks.named<JavaCompile>("compileJava") {
    options.compilerArgs.addAll(listOf("--module-path", classpath.asPath))
    classpath = files()
}

tasks.named<Test>("test") {
    enabled = false
}

val pluginPackageRoot = layout.buildDirectory.dir("jpackage")
val pluginImageDirectory = pluginPackageRoot.map { directory -> directory.dir("bu1ld-java-plugin") }
val pluginOutputDirectory = layout.buildDirectory.dir("plugin")
val pluginManifestFile = pluginOutputDirectory.map { it.file("plugin.toml") }

jlink {
    addOptions(
        "--strip-debug",
        "--compress",
        "2",
        "--no-header-files",
        "--no-man-pages",
        "--strip-native-commands"
    )

    launcher {
        name = "bu1ld-java-plugin"
    }

    jpackage {
        setImageOutputDir(pluginPackageRoot.get().asFile)
        imageName = "bu1ld-java-plugin"
        appVersion = bu1ldPluginVersion
        skipInstaller = true
        if (providers.systemProperty("os.name").get().lowercase().contains("windows")) {
            imageOptions.set(listOf("--win-console"))
        }
    }
}

val writePluginManifest by tasks.registering {
    outputs.file(pluginManifestFile)

    doLast {
        val file = pluginManifestFile.get().asFile
        file.parentFile.mkdirs()
        file.writeText(
            """
            id = "$bu1ldPluginId"
            namespace = "$bu1ldPluginNamespace"
            version = "$bu1ldPluginVersion"
            binary = "${bu1ldPluginBinary.get()}"

            [[rules]]
            name = "compile"

            [[rules]]
            name = "jar"
            """.trimIndent() + "\n",
            Charsets.UTF_8,
        )
    }
}

val jpackageImage = tasks.named("jpackageImage")

tasks.named("assemble") {
    dependsOn(jpackageImage, writePluginManifest)
}

val smokeTest by tasks.registering(JavaExec::class) {
    dependsOn(tasks.named("testClasses"))
    classpath = sourceSets["test"].runtimeClasspath
    mainClass.set("org.bu1ld.plugins.java.PluginSmokeTest")
}

tasks.named("check") {
    dependsOn(smokeTest)
}

tasks.register<Sync>("installBu1ldPlugin") {
    dependsOn(jpackageImage, writePluginManifest)
    from(pluginImageDirectory)
    from(pluginManifestFile)
    into(layout.projectDirectory.dir("../../.bu1ld/plugins/$bu1ldPluginId/$bu1ldPluginVersion"))
}
