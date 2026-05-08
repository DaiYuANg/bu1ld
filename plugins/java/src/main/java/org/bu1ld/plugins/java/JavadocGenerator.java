package org.bu1ld.plugins.java;

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
import javax.tools.DocumentationTool;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import lombok.val;
import org.apache.commons.io.FileUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class JavadocGenerator {
    private final MavenDependencyResolver dependencyResolver;

    JavadocGenerator(MavenDependencyResolver dependencyResolver) {
        this.dependencyResolver = dependencyResolver;
    }

    ExecuteResult generate(ExecuteRequest request) throws Exception {
        val spec = JavadocSpec.from(request.params());
        val workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        val outputDir = workDir.resolve(spec.out()).normalize();
        val sources = ProjectFiles.expand(workDir, spec.srcs());
        if (sources.isEmpty()) {
            FileUtils.forceMkdir(outputDir.toFile());
            return new ExecuteResult("no Java source files matched for javadoc\n");
        }

        val tool = ToolProvider.getSystemDocumentationTool();
        if (tool == null) {
            throw new IllegalStateException("JDK javadoc tool is not available in this Java runtime");
        }

        FileUtils.forceMkdir(outputDir.toFile());
        val diagnostics = new DiagnosticCollector<JavaFileObject>();
        val toolOutput = new StringWriter();
        try (val files = tool.getStandardFileManager(diagnostics, Locale.ROOT, StandardCharsets.UTF_8)) {
            val units = files.getJavaFileObjectsFromFiles(
                sources.stream().map(Path::toFile).toList()
            );
            val task = tool.getTask(
                toolOutput,
                files,
                diagnostics,
                null,
                options(workDir, outputDir, spec),
                units
            );
            if (!Boolean.TRUE.equals(task.call())) {
                var message = diagnosticsText(diagnostics);
                if (!toolOutput.toString().isBlank()) {
                    message += toolOutput;
                }
                throw new IllegalStateException("javadoc failed\n" + message.stripTrailing());
            }
        }
        return new ExecuteResult("generated javadoc for " + sources.size() + " Java source file(s) to " + spec.out() + "\n");
    }

    private List<String> options(Path workDir, Path outputDir, JavadocSpec spec) throws Exception {
        val options = new ArrayList<String>();
        options.add("-encoding");
        options.add(StandardCharsets.UTF_8.name());
        options.add("-d");
        options.add(outputDir.toString());
        if (!spec.release().isBlank()) {
            options.add("--release");
            options.add(spec.release());
        }
        val classpath = classpath(workDir, spec);
        if (!classpath.isEmpty()) {
            options.add("--class-path");
            options.add(JavaClasspath.join(classpath));
        }
        return options;
    }

    private List<Path> classpath(Path workDir, JavadocSpec spec) throws Exception {
        val entries = JavaClasspath.resolve(workDir, spec.classpath());
        entries.addAll(dependencyResolver.resolve(
            workDir,
            spec.repositories(),
            spec.dependencies(),
            spec.localRepository()
        ));
        return entries;
    }

    private String diagnosticsText(DiagnosticCollector<JavaFileObject> diagnostics) {
        val builder = new StringBuilder();
        for (val diagnostic : diagnostics.getDiagnostics()) {
            builder.append(diagnostic.getKind()).append(": ");
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

    private record JavadocSpec(
        List<String> srcs,
        List<String> classpath,
        List<String> repositories,
        List<String> dependencies,
        String out,
        String release,
        String localRepository
    ) {
        static JavadocSpec from(java.util.Map<String, Object> params) {
            val fields = new FieldMap(params);
            return new JavadocSpec(
                fields.list("srcs", JavaDefaults.SOURCES),
                fields.list("classpath", ImmutableList.of()),
                fields.list("repositories", JavaDefaults.REPOSITORIES),
                fields.list("dependencies", ImmutableList.of()),
                fields.string("out", JavaDefaults.JAVADOC_DIR),
                fields.string("release", JavaDefaults.RELEASE),
                fields.string("local_repository", JavaDefaults.LOCAL_REPOSITORY)
            );
        }
    }
}
