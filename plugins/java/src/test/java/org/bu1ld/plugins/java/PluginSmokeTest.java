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

public final class PluginSmokeTest {
    private PluginSmokeTest() {
    }

    public static void main(String[] args) throws Exception {
        Path project = Files.createTempDirectory("bu1ld-java-plugin-smoke");
        Path source = project.resolve("src/main/java/example/App.java");
        Files.createDirectories(source.getParent());
        Files.writeString(source, "package example; public final class App { public static String message() { return \"ok\"; } }\n");

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
                        "out", "build/classes/java/main",
                        "release", "17"
                    )
                ))));
            writeFrame(request, json(mapper, 4, "execute", Map.of("request", Map.of(
                    "namespace", "java",
                    "action", "jar",
                    "work_dir", project.toString(),
                    "params", Map.of(
                        "classes", "build/classes/java/main",
                        "out", "build/libs/smoke.jar"
                    )
                ))));
            ByteArrayInputStream input = new ByteArrayInputStream(request.toByteArray());
            ByteArrayOutputStream output = new ByteArrayOutputStream();

            server.serve(input, output);

            String text = output.toString(StandardCharsets.UTF_8);
            requireContains(text, "\"id\":\"org.bu1ld.java\"");
            requireContains(text, "\"namespace\":\"java\"");
            requireContains(text, "\"auto_configure\":true");
            requireContains(text, "\"name\":\"compileJava\"");
            requireContains(text, "\"kind\":\"plugin.exec\"");
            requireContains(text, "compiled 1 Java source file");
            requireContains(text, "created jar build/libs/smoke.jar");
            if (!Files.isRegularFile(project.resolve("build/libs/smoke.jar"))) {
                throw new AssertionError("jar was not created");
            }
        }
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
