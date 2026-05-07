package org.bu1ld.plugins.java;

import com.google.common.base.Joiner;
import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.File;
import java.io.IOException;
import java.io.StringWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import javax.tools.Diagnostic;
import javax.tools.DiagnosticCollector;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import org.apache.commons.io.FileUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class NativeJavaCompiler {
    ExecuteResult compile(ExecuteRequest request) throws IOException {
        CompileSpec spec = CompileSpec.from(request.params());
        Path workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        Path outputDir = workDir.resolve(spec.out()).normalize();
        List<Path> sources = ProjectFiles.expand(workDir, spec.srcs());
        if (sources.isEmpty()) {
            FileUtils.forceMkdir(outputDir.toFile());
            return new ExecuteResult("no Java source files matched\n");
        }

        JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
        if (compiler == null) {
            throw new IllegalStateException("JDK compiler is not available in this Java runtime");
        }

        FileUtils.forceMkdir(outputDir.toFile());
        DiagnosticCollector<JavaFileObject> diagnostics = new DiagnosticCollector<>();
        StringWriter compilerOutput = new StringWriter();
        try (StandardJavaFileManager files = compiler.getStandardFileManager(diagnostics, Locale.ROOT, StandardCharsets.UTF_8)) {
            Iterable<? extends JavaFileObject> units = files.getJavaFileObjectsFromFiles(
                sources.stream().map(Path::toFile).toList()
            );
            JavaCompiler.CompilationTask task = compiler.getTask(
                compilerOutput,
                files,
                diagnostics,
                compilerOptions(workDir, outputDir, spec),
                null,
                units
            );
            if (!Boolean.TRUE.equals(task.call())) {
                String message = diagnosticsText(diagnostics);
                if (!compilerOutput.toString().isBlank()) {
                    message += compilerOutput;
                }
                throw new IllegalStateException("java compile failed\n" + message.stripTrailing());
            }
        }
        return new ExecuteResult("compiled " + sources.size() + " Java source file(s) to " + spec.out() + "\n");
    }

    private List<String> compilerOptions(Path workDir, Path outputDir, CompileSpec spec) {
        List<String> options = new ArrayList<>();
        options.add("-encoding");
        options.add(StandardCharsets.UTF_8.name());
        options.add("-d");
        options.add(outputDir.toString());
        if (!spec.release().isBlank()) {
            options.add("--release");
            options.add(spec.release());
        }
        if (!spec.classpath().isEmpty()) {
            options.add("--class-path");
            options.add(classpath(workDir, spec.classpath()));
        }
        return options;
    }

    private String classpath(Path workDir, List<String> classpath) {
        List<String> entries = new ArrayList<>(classpath.size());
        for (String item : classpath) {
            entries.add(workDir.resolve(item).normalize().toString());
        }
        return Joiner.on(File.pathSeparator).join(entries);
    }

    private String diagnosticsText(DiagnosticCollector<JavaFileObject> diagnostics) {
        StringBuilder builder = new StringBuilder();
        for (Diagnostic<? extends JavaFileObject> diagnostic : diagnostics.getDiagnostics()) {
            builder
                .append(diagnostic.getKind())
                .append(": ");
            if (diagnostic.getSource() != null) {
                builder
                    .append(diagnostic.getSource().getName())
                    .append(':')
                    .append(diagnostic.getLineNumber())
                    .append(": ");
            }
            builder.append(diagnostic.getMessage(Locale.ROOT)).append('\n');
        }
        return builder.toString();
    }

    private record CompileSpec(List<String> srcs, List<String> classpath, String out, String release) {
        static CompileSpec from(java.util.Map<String, Object> params) {
            FieldMap fields = new FieldMap(params);
            return new CompileSpec(
                fields.list("srcs", ImmutableList.of("src/main/java/**/*.java")),
                fields.list("classpath", ImmutableList.of()),
                fields.string("out", "build/classes/java/main"),
                fields.string("release", "17")
            );
        }
    }
}
