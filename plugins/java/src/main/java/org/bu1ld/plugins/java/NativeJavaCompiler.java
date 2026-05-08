package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.StringWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import javax.tools.Diagnostic;
import javax.tools.DiagnosticCollector;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import lombok.val;
import org.apache.commons.io.FileUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class NativeJavaCompiler {
    private final MavenDependencyResolver dependencyResolver;

    NativeJavaCompiler(MavenDependencyResolver dependencyResolver) {
        this.dependencyResolver = dependencyResolver;
    }

    ExecuteResult compile(ExecuteRequest request) throws Exception {
        val spec = CompileSpec.from(request.params());
        val workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        val outputDir = workDir.resolve(spec.out()).normalize();
        val sources = ProjectFiles.expand(workDir, spec.srcs());
        if (sources.isEmpty()) {
            FileUtils.forceMkdir(outputDir.toFile());
            return new ExecuteResult("no Java source files matched\n");
        }

        val compiler = ToolProvider.getSystemJavaCompiler();
        if (compiler == null) {
            throw new IllegalStateException("JDK compiler is not available in this Java runtime");
        }

        FileUtils.forceMkdir(outputDir.toFile());
        val diagnostics = new DiagnosticCollector<JavaFileObject>();
        val compilerOutput = new StringWriter();
        try (val files = compiler.getStandardFileManager(diagnostics, Locale.ROOT, StandardCharsets.UTF_8)) {
            val units = files.getJavaFileObjectsFromFiles(
                sources.stream().map(Path::toFile).toList()
            );
            val task = compiler.getTask(
                compilerOutput,
                files,
                diagnostics,
                compilerOptions(workDir, outputDir, spec),
                null,
                units
            );
            if (!Boolean.TRUE.equals(task.call())) {
                var message = diagnosticsText(diagnostics);
                if (!compilerOutput.toString().isBlank()) {
                    message += compilerOutput;
                }
                throw new IllegalStateException("java compile failed\n" + message.stripTrailing());
            }
        }
        return new ExecuteResult("compiled " + sources.size() + " Java source file(s) to " + spec.out() + "\n");
    }

    private List<String> compilerOptions(Path workDir, Path outputDir, CompileSpec spec) throws Exception {
        val options = new ArrayList<String>();
        options.add("-encoding");
        options.add(StandardCharsets.UTF_8.name());
        options.add("-d");
        options.add(outputDir.toString());
        if (!spec.release().isBlank()) {
            options.add("--release");
            options.add(spec.release());
        }
        val resolved = resolvedClasspath(workDir, spec);
        if (!resolved.classpath().isEmpty()) {
            options.add("--class-path");
            options.add(JavaClasspath.join(resolved.classpath()));
        }
        if (!resolved.modulePath().isEmpty()) {
            options.add("--module-path");
            options.add(JavaClasspath.join(resolved.modulePath()));
        }
        if (!spec.addModules().isEmpty()) {
            options.add("--add-modules");
            options.add(String.join(",", spec.addModules()));
        }
        if (!resolved.processorPath().isEmpty()) {
            options.add("--processor-path");
            options.add(JavaClasspath.join(resolved.processorPath()));
        }
        if (!spec.processors().isEmpty()) {
            options.add("-processor");
            options.add(String.join(",", spec.processors()));
        }
        if (!spec.proc().isBlank()) {
            options.add("-proc:" + spec.proc());
        }
        for (val entry : spec.processorOptions().entrySet()) {
            if (entry.getValue().isBlank()) {
                options.add("-A" + entry.getKey());
                continue;
            }
            options.add("-A" + entry.getKey() + "=" + entry.getValue());
        }
        return options;
    }

    private ResolvedClasspath resolvedClasspath(Path workDir, CompileSpec spec) throws Exception {
        val classpath = JavaClasspath.resolve(workDir, spec.classpath());
        val dependencyArtifacts = dependencyResolver.resolveArtifacts(
            workDir,
            spec.repositories(),
            spec.dependencies(),
            spec.localRepository(),
            spec.offline()
        );
        classpath.addAll(dependencyArtifacts.stream().map(ResolvedArtifact::path).toList());

        val modulePath = JavaClasspath.resolve(workDir, spec.modulePath());
        val moduleArtifacts = dependencyResolver.resolveArtifacts(
            workDir,
            spec.repositories(),
            spec.moduleDependencies(),
            spec.localRepository(),
            spec.offline()
        );
        modulePath.addAll(moduleArtifacts.stream().map(ResolvedArtifact::path).toList());

        val processorPath = JavaClasspath.resolve(workDir, spec.processorPath());
        val processorArtifacts = dependencyResolver.resolveArtifacts(
            workDir,
            spec.repositories(),
            spec.annotationProcessors(),
            spec.localRepository(),
            spec.offline()
        );
        processorPath.addAll(processorArtifacts.stream().map(ResolvedArtifact::path).toList());

        val artifacts = new ArrayList<ResolvedArtifact>(
            dependencyArtifacts.size() + moduleArtifacts.size() + processorArtifacts.size()
        );
        artifacts.addAll(dependencyArtifacts);
        artifacts.addAll(moduleArtifacts);
        artifacts.addAll(processorArtifacts);
        DependencyLock.apply(workDir, spec.dependencyLock(), spec.dependencyLockMode(), artifacts);

        return new ResolvedClasspath(classpath, modulePath, processorPath);
    }

    private String diagnosticsText(DiagnosticCollector<JavaFileObject> diagnostics) {
        val builder = new StringBuilder();
        for (val diagnostic : diagnostics.getDiagnostics()) {
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

    private record CompileSpec(
        List<String> srcs,
        List<String> classpath,
        List<String> modulePath,
        List<String> repositories,
        List<String> dependencies,
        List<String> moduleDependencies,
        List<String> addModules,
        List<String> processorPath,
        List<String> annotationProcessors,
        List<String> processors,
        Map<String, String> processorOptions,
        String proc,
        String out,
        String release,
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline
    ) {
        static CompileSpec from(java.util.Map<String, Object> params) {
            val fields = new FieldMap(params);
            return new CompileSpec(
                fields.list("srcs", JavaDefaults.SOURCES),
                fields.list("classpath", ImmutableList.of()),
                fields.list("module_path", ImmutableList.of()),
                fields.list("repositories", JavaDefaults.REPOSITORIES),
                fields.list("dependencies", ImmutableList.of()),
                fields.list("module_dependencies", ImmutableList.of()),
                fields.list("add_modules", ImmutableList.of()),
                fields.list("processor_path", ImmutableList.of()),
                fields.list("annotation_processors", ImmutableList.of()),
                fields.list("processors", ImmutableList.of()),
                fields.stringMap("processor_options", Map.of()),
                fields.string("proc", ""),
                fields.string("out", JavaDefaults.CLASSES_DIR),
                fields.string("release", JavaDefaults.RELEASE),
                fields.string("local_repository", JavaDefaults.LOCAL_REPOSITORY),
                fields.string("dependency_lock", ""),
                fields.string("dependency_lock_mode", DependencyLock.MODE_OFF),
                fields.bool("offline", false)
            );
        }
    }

    private record ResolvedClasspath(List<Path> classpath, List<Path> modulePath, List<Path> processorPath) {
    }
}
