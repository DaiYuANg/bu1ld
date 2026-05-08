package org.bu1ld.plugins.java;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.avaje.inject.BeanScope;
import java.io.File;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import java.util.regex.Pattern;
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
            import com.example.GenerateGreeting;
            /** Smoke test app. */
            @GenerateGreeting("ok")
            public final class App {
                /** Returns the smoke test message. */
                public static String message() { return GeneratedGreeting.message(); }
            }
            """);
        Path resource = project.resolve("src/main/resources/app.properties");
        Files.createDirectories(resource.getParent());
        Files.writeString(resource, "message=ok\n");
        Path testSource = project.resolve("src/test/java/example/AppTest.java");
        Files.createDirectories(testSource.getParent());
        Files.writeString(testSource, """
            package example;
            import org.junit.jupiter.api.Test;
            import static org.junit.jupiter.api.Assertions.assertEquals;
            final class AppTest {
                @Test
                void messageUsesGeneratedGreeting() {
                    assertEquals("ok", App.message());
                }
            }
            """);

        List<String> junitClasspath = junitRuntimeClasspath();
        ObjectMapper mapper = new ObjectMapper();
        try (BeanScope scope = BeanScope.builder().build()) {
            Server server = scope.get(Server.class);
            ByteArrayOutputStream request = new ByteArrayOutputStream();
            writeFrame(request, json(mapper, 1, "metadata", null));
            writeFrame(request, json(mapper, 2, "configure", Map.of("config", Map.of(
                    "namespace", "java",
                    "fields", Map.of(
                        "name", "smoke",
                        "release", "17",
                        "dependency_lock", "build/bu1ld-java.lock",
                        "dependency_lock_mode", "write",
                        "source_sets", Map.of("test", Map.of())
                    )
                ))));
            writeFrame(request, json(mapper, 3, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "compile",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "srcs", List.of("src/main/java/**/*.java"),
                        "repositories", List.of(repository.toUri().toString()),
                        "dependencies", List.of("com.example:codegen:1.0.0"),
                        "annotation_processors", List.of("com.example:codegen:1.0.0"),
                        "local_repository", "build/dependency-cache/maven",
                        "dependency_lock", "build/bu1ld-java.lock",
                        "dependency_lock_mode", "write",
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
                    "action", "compile",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "srcs", List.of("src/test/java/**/*.java"),
                        "classpath", concat(List.of("build/classes/java/main", "build/resources/main"), junitClasspath),
                        "out", "build/classes/java/test",
                        "release", "17"
                    )
                ))));
            writeFrame(request, json(mapper, 6, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "test",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "classes", List.of("build/classes/java/test"),
                        "classpath", concat(List.of("build/classes/java/main", "build/resources/main"), junitClasspath),
                        "launcher_dependencies", List.of(),
                        "reports_dir", "build/test-results/test",
                        "include_engines", List.of("junit-jupiter")
                    )
                ))));
            writeFrame(request, json(mapper, 7, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "roots", List.of("build/classes/java/main", "build/resources/main"),
                        "out", "build/libs/smoke.jar"
                    )
                ))));
            writeFrame(request, json(mapper, 8, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "javadoc",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "srcs", List.of("src/main/java/**/*.java"),
                        "classpath", List.of("build/classes/java/main"),
                        "repositories", List.of(repository.toUri().toString()),
                        "dependencies", List.of("com.example:codegen:1.0.0"),
                        "local_repository", "build/dependency-cache/maven",
                        "dependency_lock", "build/bu1ld-java.lock",
                        "dependency_lock_mode", "read",
                        "out", "build/docs/javadoc",
                        "release", "17"
                    )
                ))));
            writeFrame(request, json(mapper, 9, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "roots", List.of("src/main/java", "src/main/resources"),
                        "out", "build/libs/smoke-sources.jar"
                    )
                ))));
            writeFrame(request, json(mapper, 10, "execute", Map.of("request", Map.of(
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
            requireContains(text, "\"name\":\"compileTestJava\"");
            requireContains(text, "\"name\":\"testClasses\"");
            requireContains(text, "\"name\":\"test\"");
            requireContains(text, "\"kind\":\"plugin.exec\"");
            requireContains(text, "compiled 1 Java source file");
            requireContains(text, "processed 1 Java resource file");
            requireContains(text, "ran 1 Java test");
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
            if (!Files.isRegularFile(project.resolve("build/test-results/test/summary.txt"))) {
                throw new AssertionError("test summary was not generated");
            }
            requireContains(Files.readString(project.resolve("build/bu1ld-java.lock")), "com.example:codegen:jar:1.0.0");
        }
    }

    private static Path createLocalMavenRepository(Path project) throws Exception {
        Path annotationSource = project.resolve("codegen-src/com/example/GenerateGreeting.java");
        Path processorSource = project.resolve("codegen-src/com/example/GenerateGreetingProcessor.java");
        Path processorClasses = project.resolve("codegen-classes");
        Files.createDirectories(annotationSource.getParent());
        Files.createDirectories(processorClasses);
        Files.writeString(annotationSource, """
            package com.example;
            import java.lang.annotation.ElementType;
            import java.lang.annotation.Retention;
            import java.lang.annotation.RetentionPolicy;
            import java.lang.annotation.Target;
            /** Marks a type for smoke-test code generation. */
            @Retention(RetentionPolicy.SOURCE)
            @Target(ElementType.TYPE)
            public @interface GenerateGreeting {
                String value() default "ok";
            }
            """);
        Files.writeString(processorSource, """
            package com.example;
            import java.io.IOException;
            import java.io.UncheckedIOException;
            import java.util.Set;
            import javax.annotation.processing.AbstractProcessor;
            import javax.annotation.processing.RoundEnvironment;
            import javax.annotation.processing.SupportedAnnotationTypes;
            import javax.annotation.processing.SupportedSourceVersion;
            import javax.lang.model.SourceVersion;
            import javax.lang.model.element.Element;
            import javax.lang.model.element.TypeElement;

            @SupportedAnnotationTypes("com.example.GenerateGreeting")
            @SupportedSourceVersion(SourceVersion.RELEASE_17)
            public final class GenerateGreetingProcessor extends AbstractProcessor {
                @Override
                public boolean process(Set<? extends TypeElement> annotations, RoundEnvironment roundEnv) {
                    if (roundEnv.processingOver()) {
                        return false;
                    }
                    for (Element element : roundEnv.getElementsAnnotatedWith(GenerateGreeting.class)) {
                        GenerateGreeting annotation = element.getAnnotation(GenerateGreeting.class);
                        try (var writer = processingEnv.getFiler()
                            .createSourceFile("example.GeneratedGreeting", element)
                            .openWriter()) {
                            writer.write("package example;\\n");
                            writer.write("public final class GeneratedGreeting {\\n");
                            writer.write("  private GeneratedGreeting() {}\\n");
                            writer.write("  public static String message() { return \\"");
                            writer.write(annotation.value());
                            writer.write("\\"; }\\n");
                            writer.write("}\\n");
                        } catch (IOException e) {
                            throw new UncheckedIOException(e);
                        }
                    }
                    return false;
                }
            }
            """);

        int compiled = ToolProvider.getSystemJavaCompiler().run(
            null,
            null,
            null,
            "-d",
            processorClasses.toString(),
            annotationSource.toString(),
            processorSource.toString()
        );
        if (compiled != 0) {
            throw new AssertionError("processor dependency failed to compile");
        }

        Path services = processorClasses.resolve("META-INF/services/javax.annotation.processing.Processor");
        Files.createDirectories(services.getParent());
        Files.writeString(services, "com.example.GenerateGreetingProcessor\n");

        Path artifactDir = project.resolve("repo/com/example/codegen/1.0.0");
        Files.createDirectories(artifactDir);
        Path jar = artifactDir.resolve("codegen-1.0.0.jar");
        try (JarOutputStream output = new JarOutputStream(Files.newOutputStream(jar))) {
            addJarEntry(output, processorClasses, "com/example/GenerateGreeting.class");
            addJarEntry(output, processorClasses, "com/example/GenerateGreetingProcessor.class");
            addJarEntry(output, processorClasses, "META-INF/services/javax.annotation.processing.Processor");
        }
        Files.writeString(artifactDir.resolve("codegen-1.0.0.pom"), """
            <project xmlns="http://maven.apache.org/POM/4.0.0">
              <modelVersion>4.0.0</modelVersion>
              <groupId>com.example</groupId>
              <artifactId>codegen</artifactId>
              <version>1.0.0</version>
            </project>
            """);
        return project.resolve("repo");
    }

    private static void addJarEntry(JarOutputStream output, Path root, String name) throws Exception {
        output.putNextEntry(new JarEntry(name));
        Files.copy(root.resolve(name), output);
        output.closeEntry();
    }

    private static List<String> junitRuntimeClasspath() {
        List<String> entries = Arrays.stream(System.getProperty("java.class.path").split(Pattern.quote(File.pathSeparator)))
            .filter(PluginSmokeTest::isJUnitRuntimeEntry)
            .toList();
        if (entries.isEmpty()) {
            throw new AssertionError("JUnit runtime classpath was not available to smoke test");
        }
        return entries;
    }

    private static boolean isJUnitRuntimeEntry(String entry) {
        String name = Path.of(entry).getFileName().toString();
        return name.startsWith("junit-")
            || name.startsWith("opentest4j-")
            || name.startsWith("apiguardian-api-");
    }

    private static List<String> concat(List<String> first, List<String> second) {
        List<String> values = new ArrayList<>(first.size() + second.size());
        values.addAll(first);
        values.addAll(second);
        return values;
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
