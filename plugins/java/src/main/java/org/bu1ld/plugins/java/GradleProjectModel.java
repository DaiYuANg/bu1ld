package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.File;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashSet;
import lombok.val;

@Component
final class GradleProjectModel {
    boolean available(Path root) {
        return Files.isRegularFile(root.resolve("build.gradle"))
            || Files.isRegularFile(root.resolve("build.gradle.kts"))
            || Files.isRegularFile(root.resolve("settings.gradle"))
            || Files.isRegularFile(root.resolve("settings.gradle.kts"));
    }

    ImmutableList<String> tasks(Path root, ImmutableList<String> fallback) {
        if (!available(root)) {
            return ImmutableList.of();
        }
        val names = new LinkedHashSet<String>();
        try {
            val connector = type("org.gradle.tooling.GradleConnector")
                .getMethod("newConnector")
                .invoke(null);
            method(connector.getClass(), "forProjectDirectory", File.class).invoke(connector, root.toFile());
            callOptional(connector, "useBuildDistribution");
            val connection = method(connector.getClass(), "connect").invoke(connector);
            try {
                val model = method(connection.getClass(), "getModel", Class.class)
                    .invoke(connection, type("org.gradle.tooling.model.gradle.BuildInvocations"));
                collectNames(names, method(model.getClass(), "getTaskSelectors").invoke(model));
                collectNames(names, method(model.getClass(), "getTasks").invoke(model));
            } finally {
                method(connection.getClass(), "close").invoke(connection);
            }
        } catch (Exception ignored) {
            names.addAll(fallback);
        }
        if (names.isEmpty()) {
            names.addAll(fallback);
        }
        return ImmutableList.copyOf(names);
    }

    private void collectNames(LinkedHashSet<String> names, Object values) throws Exception {
        if (!(values instanceof Iterable<?> items)) {
            return;
        }
        for (val item : items) {
            val name = method(item.getClass(), "getName").invoke(item);
            if (name instanceof String text && !text.isBlank()) {
                names.add(text);
            }
        }
    }

    private void callOptional(Object target, String name) throws Exception {
        try {
            method(target.getClass(), name).invoke(target);
        } catch (NoSuchMethodException ignored) {
            // Older Tooling API versions do not expose every connector option.
        }
    }

    private static Class<?> type(String name) throws ClassNotFoundException {
        return Class.forName(name);
    }

    private static Method method(Class<?> type, String name, Class<?>... parameterTypes) throws NoSuchMethodException {
        return type.getMethod(name, parameterTypes);
    }
}
