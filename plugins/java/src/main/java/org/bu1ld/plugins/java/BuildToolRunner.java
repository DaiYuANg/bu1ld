package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.OutputStream;
import java.io.PrintStream;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.List;
import lombok.val;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class BuildToolRunner {
    ExecuteResult runGradle(ExecuteRequest request) throws Exception {
        val args = actionArgs(request);
        if (args.isEmpty()) {
            throw new IllegalArgumentException("gradle action requires at least one task");
        }
        val output = new ByteArrayOutputStream();
        val connector = type("org.gradle.tooling.GradleConnector")
            .getMethod("newConnector")
            .invoke(null);
        method(connector.getClass(), "forProjectDirectory", File.class)
            .invoke(connector, Path.of(request.workDir()).toFile());
        callOptional(connector, "useBuildDistribution");
        val connection = method(connector.getClass(), "connect").invoke(connector);
        try {
            val launcher = method(connection.getClass(), "newBuild").invoke(connection);
            method(launcher.getClass(), "forTasks", String[].class).invoke(launcher, (Object) args.toArray(String[]::new));
            method(launcher.getClass(), "setStandardOutput", OutputStream.class).invoke(launcher, output);
            method(launcher.getClass(), "setStandardError", OutputStream.class).invoke(launcher, output);
            method(launcher.getClass(), "run").invoke(launcher);
            return toolResult("gradle", args, output);
        } catch (InvocationTargetException error) {
            throw toolFailure("gradle", args, output, error);
        } finally {
            method(connection.getClass(), "close").invoke(connection);
        }
    }

    ExecuteResult runMaven(ExecuteRequest request) throws Exception {
        val args = actionArgs(request);
        if (args.isEmpty()) {
            throw new IllegalArgumentException("maven action requires at least one goal");
        }
        val output = new ByteArrayOutputStream();
        val root = Path.of(request.workDir()).toAbsolutePath().normalize().toString();
        val previousRoot = System.getProperty("maven.multiModuleProjectDirectory");
        System.setProperty("maven.multiModuleProjectDirectory", root);
        try (val stream = new PrintStream(output, true, StandardCharsets.UTF_8)) {
            val cli = type("org.apache.maven.cli.MavenCli").getConstructor().newInstance();
            val exit = (Integer) method(cli.getClass(), "doMain", String[].class, String.class, PrintStream.class, PrintStream.class)
                .invoke(cli, (Object) args.toArray(String[]::new), root, stream, stream);
            val text = output.toString(StandardCharsets.UTF_8);
            if (exit != 0) {
                throw new IllegalStateException("embedded maven " + String.join(" ", args)
                    + " exited with code " + exit + "\n" + text.strip());
            }
            return toolResult("embedded maven", args, output);
        } catch (InvocationTargetException error) {
            throw toolFailure("embedded maven", args, output, error);
        } finally {
            if (previousRoot == null) {
                System.clearProperty("maven.multiModuleProjectDirectory");
            } else {
                System.setProperty("maven.multiModuleProjectDirectory", previousRoot);
            }
        }
    }

    private List<String> actionArgs(ExecuteRequest request) {
        val fields = new FieldMap(request.params());
        return fields.list("args", ImmutableList.of());
    }

    private Exception toolFailure(String tool, List<String> args, ByteArrayOutputStream output, InvocationTargetException error) {
        val cause = error.getCause() == null ? error : error.getCause();
        return new IllegalStateException(tool + " " + String.join(" ", args)
            + " failed: " + cause.getMessage() + "\n" + output.toString(StandardCharsets.UTF_8).strip(), cause);
    }

    private ExecuteResult toolResult(String tool, List<String> args, ByteArrayOutputStream output) {
        val text = output.toString(StandardCharsets.UTF_8);
        if (!text.isBlank()) {
            return new ExecuteResult(text);
        }
        return new ExecuteResult("ran " + tool + " " + String.join(" ", args) + "\n");
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
