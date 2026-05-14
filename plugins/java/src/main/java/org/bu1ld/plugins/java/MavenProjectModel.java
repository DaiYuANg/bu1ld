package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.Reader;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Optional;
import lombok.val;

@Component
final class MavenProjectModel {
    Optional<Project> load(Path root) {
        val pom = root.resolve("pom.xml");
        if (!Files.isRegularFile(pom)) {
            return Optional.empty();
        }
        try (Reader reader = Files.newBufferedReader(pom)) {
            val parser = type("org.apache.maven.model.io.xpp3.MavenXpp3Reader").getConstructor().newInstance();
            val model = method(parser.getClass(), "read", Reader.class).invoke(parser, reader);
            val artifactId = stringValue(model, "getArtifactId", root.getFileName().toString());
            val packaging = stringValue(model, "getPackaging", "jar");
            return Optional.of(new Project(artifactId, packaging));
        } catch (Exception error) {
            throw new IllegalArgumentException("read Maven project model from " + pom + ": " + error.getMessage(), error);
        }
    }

    ImmutableList<String> lifecycleGoals(Project project, ImmutableList<String> fallback) {
        if ("pom".equals(project.packaging())) {
            return ImmutableList.of("validate", "verify", "install");
        }
        return fallback;
    }

    private String stringValue(Object target, String method, String fallback) throws Exception {
        val value = method(target.getClass(), method).invoke(target);
        if (value instanceof String text && !text.isBlank()) {
            return text;
        }
        return fallback;
    }

    private static Class<?> type(String name) throws ClassNotFoundException {
        return Class.forName(name);
    }

    private static Method method(Class<?> type, String name, Class<?>... parameterTypes) throws NoSuchMethodException {
        return type.getMethod(name, parameterTypes);
    }

    record Project(String artifactId, String packaging) {
    }
}
