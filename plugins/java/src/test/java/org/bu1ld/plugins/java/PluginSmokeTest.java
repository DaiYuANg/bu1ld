package org.bu1ld.plugins.java;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.avaje.inject.BeanScope;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import javax.tools.ToolProvider;

public final class PluginSmokeTest {
    private PluginSmokeTest() {
    }

    public static void main(String[] args) throws Exception {
        Path project = Files.createTempDirectory("bu1ld-java-plugin-smoke");
        Path repository = createLocalMavenRepository(project);
        Path source = project.resolve("src/main/java/example/App.java");
        Files.createDirectories(source.getParent());
        Files.writeString(source, """
            package example;
            import com.example.Helper;
            /** Smoke test app. */
            public final class App {
                /** Returns the smoke test message. */
                public static String message() { return Helper.message(); }
            }
            """);
        Path resource = project.resolve("src/main/resources/app.properties");
        Files.createDirectories(resource.getParent());
        Files.writeString(resource, "message=ok\n");

        ObjectMapper mapper = new ObjectMapper();
        try (BeanScope scope = BeanScope.builder().build()) {
            Server server = scope.get(Server.class);
            ByteArrayOutputStream request = new ByteArrayOutputStream();
            writeFrame(request, json(mapper, 1, "metadata", null));
            writeFrame(request, json(mapper, 2, "configure", Map.of("config", Map.of(
                    "namespace", "java",
                    "fields", Map.of("name", "smoke", "release", "17")
                ))));
            writeFrame(request, json(mapper, 3, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "compile",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "srcs", List.of("src/main/java/**/*.java"),
                        "repositories", List.of(repository.toUri().toString()),
                        "dependencies", List.of("com.example:helper:1.0.0"),
                        "local_repository", "build/dependency-cache/maven",
                        "out", "build/classes/java/main",
                        "release", "17"
                    )
                ))));
            writeFrame(request, json(mapper, 4, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "resources",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "resources", List.of("src/main/resources/**"),
                        "resource_roots", List.of("src/main/resources"),
                        "out", "build/resources/main"
                    )
                ))));
            writeFrame(request, json(mapper, 5, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "roots", List.of("build/classes/java/main", "build/resources/main"),
                        "out", "build/libs/smoke.jar"
                    )
                ))));
            writeFrame(request, json(mapper, 6, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "javadoc",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "srcs", List.of("src/main/java/**/*.java"),
                        "repositories", List.of(repository.toUri().toString()),
                        "dependencies", List.of("com.example:helper:1.0.0"),
                        "local_repository", "build/dependency-cache/maven",
                        "out", "build/docs/javadoc",
                        "release", "17"
                    )
                ))));
            writeFrame(request, json(mapper, 7, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "roots", List.of("src/main/java", "src/main/resources"),
                        "out", "build/libs/smoke-sources.jar"
                    )
                ))));
            writeFrame(request, json(mapper, 8, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "roots", List.of("build/docs/javadoc"),
                        "out", "build/libs/smoke-javadoc.jar"
                    )
                ))));
            ByteArrayInputStream input = new ByteArrayInputStream(request.toByteArray());
            ByteArrayOutputStream output = new ByteArrayOutputStream();

            server.serve(input, output);

            String text = output.toString(StandardCharsets.UTF_8);
            requireContains(text, "\"id\":\"org.bu1ld.java\"");
            requireContains(text, "\"namespace\":\"java\"");
            requireContains(text, "\"protocol_version\":1");
            requireContains(text, "\"capabilities\":[\"metadata\",\"expand\",\"configure\",\"execute\"]");
            requireContains(text, "\"auto_configure\":true");
            requireContains(text, "\"name\":\"compileJava\"");
            requireContains(text, "\"name\":\"processResources\"");
            requireContains(text, "\"name\":\"javadoc\"");
            requireContains(text, "\"name\":\"sourcesJar\"");
            requireContains(text, "\"name\":\"javadocJar\"");
            requireContains(text, "\"kind\":\"plugin.exec\"");
            requireContains(text, "compiled 1 Java source file");
            requireContains(text, "processed 1 Java resource file");
            requireContains(text, "created jar build/libs/smoke.jar");
            requireContains(text, "generated javadoc for 1 Java source file");
            requireContains(text, "created jar build/libs/smoke-sources.jar");
            requireContains(text, "created jar build/libs/smoke-javadoc.jar");
            if (!Files.isRegularFile(project.resolve("build/libs/smoke.jar"))) {
                throw new AssertionError("jar was not created");
            }
            if (!Files.isRegularFile(project.resolve("build/libs/smoke-sources.jar"))) {
                throw new AssertionError("sources jar was not created");
            }
            if (!Files.isRegularFile(project.resolve("build/libs/smoke-javadoc.jar"))) {
                throw new AssertionError("javadoc jar was not created");
            }
            if (!Files.isRegularFile(project.resolve("build/resources/main/app.properties"))) {
                throw new AssertionError("resources were not processed");
            }
            if (!Files.isRegularFile(project.resolve("build/docs/javadoc/example/App.html"))) {
                throw new AssertionError("javadoc was not generated");
            }
        }
    }

    private static Path createLocalMavenRepository(Path project) throws Exception {
        Path helperSource = project.resolve("helper-src/com/example/Helper.java");
        Path helperClasses = project.resolve("helper-classes");
        Files.createDirectories(helperSource.getParent());
        Files.createDirectories(helperClasses);
        Files.writeString(helperSource, """
            package com.example;
            /** Smoke test dependency. */
            public final class Helper {
                /** Returns the smoke test dependency message. */
                public static String message() { return "ok"; }
            }
            """);

        int compiled = ToolProvider.getSystemJavaCompiler().run(
            null,
            null,
            null,
            "-d",
            helperClasses.toString(),
            helperSource.toString()
        );
        if (compiled != 0) {
            throw new AssertionError("helper dependency failed to compile");
        }

        Path artifactDir = project.resolve("repo/com/example/helper/1.0.0");
        Files.createDirectories(artifactDir);
        Path jar = artifactDir.resolve("helper-1.0.0.jar");
        try (JarOutputStream output = new JarOutputStream(Files.newOutputStream(jar))) {
            Path helperClass = helperClasses.resolve("com/example/Helper.class");
            output.putNextEntry(new JarEntry("com/example/Helper.class"));
            Files.copy(helperClass, output);
            output.closeEntry();
        }
        Files.writeString(artifactDir.resolve("helper-1.0.0.pom"), """
            <project xmlns="http://maven.apache.org/POM/4.0.0">
              <modelVersion>4.0.0</modelVersion>
              <groupId>com.example</groupId>
              <artifactId>helper</artifactId>
              <version>1.0.0</version>
            </project>
            """);
        return project.resolve("repo");
    }

    private static String json(ObjectMapper mapper, long id, String method, Object params) throws Exception {
        if (params == null) {
            return mapper.writeValueAsString(Map.of("jsonrpc", "2.0", "id", id, "method", method));
        }
        return mapper.writeValueAsString(Map.of("jsonrpc", "2.0", "id", id, "method", method, "params", params));
    }

    private static void writeFrame(ByteArrayOutputStream output, String json) {
        byte[] payload = json.getBytes(StandardCharsets.UTF_8);
        output.writeBytes(("Content-Length: " + payload.length + "\r\n\r\n").getBytes(StandardCharsets.UTF_8));
        output.writeBytes(payload);
    }

    private static void requireContains(String text, String want) {
        if (!text.contains(want)) {
            throw new AssertionError("output missing " + want + ": " + text);
        }
    }
}
