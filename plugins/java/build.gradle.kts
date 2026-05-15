plugins {
    alias(libs.plugins.jlink)
    alias(libs.plugins.lombok)
    application
}

group = "org.bu1ld.plugins"
version = "0.1.4"

repositories {
    mavenCentral()
    maven {
        url = uri("https://repo.gradle.org/gradle/libs-releases")
    }
}

val javaReleaseVersion = libs.versions.javaRelease.get()
val bu1ldPluginId = "org.bu1ld.java"
val bu1ldPluginNamespace = "java"
val bu1ldPluginVersion = version.toString()
val bu1ldJpackageAppVersion = jpackageCompatibleAppVersion(bu1ldPluginVersion)
val currentOsName = providers.systemProperty("os.name").get().lowercase()
val currentOsIsWindows = currentOsName.contains("windows")
val currentOsIsMac = currentOsName.contains("mac") || currentOsName.contains("darwin")
val bu1ldPluginBinary = when {
    currentOsIsWindows -> "bu1ld-java-plugin.exe"
    currentOsIsMac -> "bu1ld-java-plugin.app/Contents/MacOS/bu1ld-java-plugin"
    else -> "bin/bu1ld-java-plugin"
}

fun jpackageCompatibleAppVersion(value: String): String {
    val numericParts = value
        .substringBefore("-")
        .substringBefore("+")
        .split(".")
        .take(3)
        .map { part -> part.toIntOrNull() ?: 0 }
        .toMutableList()
    while (numericParts.size < 3) {
        numericParts.add(0)
    }
    if (numericParts[0] <= 0) {
        numericParts[0] = 1
    }
    return numericParts.take(3).joinToString(".")
}

fun File.isEmbeddedToolRuntimeJar(): Boolean {
    val jarName = name
    return jarName.startsWith("maven-") ||
        jarName.startsWith("gradle-") ||
        jarName.startsWith("plexus-") ||
        jarName.startsWith("sisu-") ||
        jarName.startsWith("guice-") ||
        jarName.startsWith("aopalliance-") ||
        jarName.startsWith("javax.annotation-") ||
        jarName.startsWith("httpclient-") ||
        jarName.startsWith("httpcore-") ||
        jarName.startsWith("commons-codec-") ||
        jarName.startsWith("commons-cli-") ||
        jarName.startsWith("jcl-over-slf4j-") ||
        jarName.startsWith("javax.inject-") ||
        jarName.startsWith("slf4j-")
}

fun File.isMavenResolverSupplierJar(): Boolean =
    name.startsWith("maven-resolver-supplier")

java {
    modularity.inferModulePath.set(true)
}

dependencies {
    implementation(libs.lsp4jJsonrpc)
    implementation(libs.commonsLang3)
    implementation(libs.commonsIo)
    implementation(libs.guava)
    implementation(libs.avajeInject)
    implementation(libs.gradleToolingApi)
    implementation(libs.mavenEmbedder)
    implementation(libs.mavenCompat)
    implementation(libs.mavenResolverSupplierMvn3)
    implementation(libs.javaxInject)
    runtimeOnly(libs.slf4jNop)
    annotationProcessor(libs.avajeInjectGenerator)
    testImplementation(libs.jacksonDatabind)
    testImplementation(libs.junitJupiterApi)
    testRuntimeOnly(libs.junitJupiterEngine)
    testRuntimeOnly(libs.junitPlatformLauncher)
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
    val embeddedToolClasspath = classpath.filter {
        it.isEmbeddedToolRuntimeJar() && !it.isMavenResolverSupplierJar()
    }
    val modulePath = classpath.filter {
        !it.isEmbeddedToolRuntimeJar() || it.isMavenResolverSupplierJar()
    }
    options.compilerArgs.addAll(listOf("--module-path", modulePath.asPath))
    classpath = embeddedToolClasspath
}

tasks.named<Test>("test") {
    enabled = false
}

val pluginPackageRoot = layout.buildDirectory.dir("jpackage")
val pluginImageName = if (currentOsIsMac) "bu1ld-java-plugin.app" else "bu1ld-java-plugin"
val pluginImageDirectory = pluginPackageRoot.map { directory -> directory.dir(pluginImageName) }
val pluginOutputDirectory = layout.buildDirectory.dir("plugin")
val generatedPluginManifestFile = layout.buildDirectory.file("generated/plugin/plugin.toml")

jlink {
    forceMerge(
        "maven-",
        "gradle-",
        "plexus-",
        "sisu-",
        "guice-",
        "aopalliance-",
        "javax.annotation-",
        "httpclient-",
        "httpcore-",
        "commons-codec-",
        "commons-cli-",
        "jcl-over-slf4j-",
        "javax.inject-",
        "slf4j-"
    )
    addExtraDependencies("commons-io")

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
        appVersion = bu1ldJpackageAppVersion
        skipInstaller = true
        if (currentOsIsWindows) {
            imageOptions.set(listOf("--win-console"))
        }
    }
}

val writePluginManifest by tasks.registering {
    outputs.file(generatedPluginManifestFile)

    doLast {
        val file = generatedPluginManifestFile.get().asFile
        file.parentFile.mkdirs()
        file.writeText(
            """
            id = "$bu1ldPluginId"
            namespace = "$bu1ldPluginNamespace"
            version = "$bu1ldPluginVersion"
            binary = "$bu1ldPluginBinary"

            [[rules]]
            name = "compile"

            [[rules]]
            name = "resources"

            [[rules]]
            name = "jar"

            [[rules]]
            name = "javadoc"

            [[rules]]
            name = "test"

            [[rules]]
            name = "gradle"

            [[rules]]
            name = "maven"
            """.trimIndent() + "\n",
            Charsets.UTF_8,
        )
    }
}

val jpackageImage = tasks.named("jpackageImage")

val stageBu1ldPlugin by tasks.registering(Sync::class) {
    dependsOn(jpackageImage, writePluginManifest)
    if (currentOsIsMac) {
        from(pluginImageDirectory) {
            into(pluginImageName)
        }
    } else {
        from(pluginImageDirectory)
    }
    from(generatedPluginManifestFile)
    into(pluginOutputDirectory)
}

tasks.named("assemble") {
    dependsOn(stageBu1ldPlugin)
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
    dependsOn(stageBu1ldPlugin)
    from(pluginOutputDirectory)
    into(layout.projectDirectory.dir("../../.bu1ld/plugins/$bu1ldPluginId/$bu1ldPluginVersion"))
}
