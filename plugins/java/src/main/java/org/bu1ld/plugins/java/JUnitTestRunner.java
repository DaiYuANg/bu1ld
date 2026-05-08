package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.io.PrintWriter;
import java.io.StringWriter;
import java.lang.reflect.Array;
import java.lang.reflect.Method;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import lombok.val;
import org.apache.commons.io.FileUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class JUnitTestRunner {
    private final MavenDependencyResolver dependencyResolver;

    JUnitTestRunner(MavenDependencyResolver dependencyResolver) {
        this.dependencyResolver = dependencyResolver;
    }

    ExecuteResult run(ExecuteRequest request) throws Exception {
        val spec = TestSpec.from(request.params());
        val workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        val reportsDir = workDir.resolve(spec.reportsDir()).normalize();
        val classRoots = JavaClasspath.resolve(workDir, spec.classes());
        val classpath = runtimeClasspath(workDir, spec, classRoots);

        FileUtils.forceMkdir(reportsDir.toFile());
        val previousLoader = Thread.currentThread().getContextClassLoader();
        try (val loader = new URLClassLoader(urls(classpath), ClassLoader.getPlatformClassLoader())) {
            Thread.currentThread().setContextClassLoader(loader);
            val runtime = JUnitRuntime.load(loader);
            val listener = runtime.newSummaryListener();
            runtime.execute(discoveryRequest(runtime, classRoots, spec), listener);
            val summary = runtime.summary(listener);
            val text = runtime.summaryText(summary);
            Files.writeString(reportsDir.resolve("summary.txt"), text, StandardCharsets.UTF_8);
            if (spec.failIfNoTests() && runtime.longValue(summary, "getTestsFoundCount") == 0) {
                throw new IllegalStateException("no Java tests were found\n" + text.stripTrailing());
            }
            if (runtime.longValue(summary, "getTestsFailedCount") > 0) {
                throw new IllegalStateException("java tests failed\n" + text.stripTrailing());
            }
            return new ExecuteResult(
                "ran " + runtime.longValue(summary, "getTestsFoundCount") + " Java test(s): "
                    + runtime.longValue(summary, "getTestsSucceededCount") + " succeeded, "
                    + runtime.longValue(summary, "getTestsFailedCount") + " failed, "
                    + runtime.longValue(summary, "getTestsSkippedCount") + " skipped\n"
            );
        } finally {
            Thread.currentThread().setContextClassLoader(previousLoader);
        }
    }

    private List<Path> runtimeClasspath(Path workDir, TestSpec spec, List<Path> classRoots) throws Exception {
        val entries = new ArrayList<Path>();
        entries.addAll(classRoots);
        entries.addAll(JavaClasspath.resolve(workDir, spec.classpath()));

        val runtimeArtifacts = dependencyResolver.resolveArtifacts(
            workDir,
            spec.repositories(),
            spec.dependencies(),
            spec.localRepository(),
            spec.offline()
        );
        entries.addAll(runtimeArtifacts.stream().map(ResolvedArtifact::path).toList());

        val launcherArtifacts = dependencyResolver.resolveArtifacts(
            workDir,
            spec.repositories(),
            spec.launcherDependencies(),
            spec.localRepository(),
            spec.offline()
        );
        entries.addAll(launcherArtifacts.stream().map(ResolvedArtifact::path).toList());

        val artifacts = new ArrayList<ResolvedArtifact>(runtimeArtifacts.size() + launcherArtifacts.size());
        artifacts.addAll(runtimeArtifacts);
        artifacts.addAll(launcherArtifacts);
        DependencyLock.apply(workDir, spec.dependencyLock(), spec.dependencyLockMode(), artifacts);

        return ImmutableList.copyOf(new LinkedHashSet<>(entries));
    }

    private Object discoveryRequest(JUnitRuntime runtime, List<Path> classRoots, TestSpec spec) throws Exception {
        val builder = runtime.requestBuilder();
        runtime.selectClasspathRoots(builder, classRoots);
        val filters = new ArrayList<Object>();
        if (!spec.includeTags().isEmpty()) {
            filters.add(runtime.filter("org.junit.platform.launcher.TagFilter", "includeTags", spec.includeTags()));
        }
        if (!spec.excludeTags().isEmpty()) {
            filters.add(runtime.filter("org.junit.platform.launcher.TagFilter", "excludeTags", spec.excludeTags()));
        }
        if (!spec.includeEngines().isEmpty()) {
            filters.add(runtime.filter("org.junit.platform.launcher.EngineFilter", "includeEngines", spec.includeEngines()));
        }
        if (!spec.excludeEngines().isEmpty()) {
            filters.add(runtime.filter("org.junit.platform.launcher.EngineFilter", "excludeEngines", spec.excludeEngines()));
        }
        runtime.filters(builder, filters);
        return runtime.buildRequest(builder);
    }

    private URL[] urls(List<Path> classpath) throws Exception {
        val urls = new URL[classpath.size()];
        for (int i = 0; i < classpath.size(); i++) {
            urls[i] = classpath.get(i).toUri().toURL();
        }
        return urls;
    }

    private record TestSpec(
        List<String> classes,
        List<String> classpath,
        List<String> repositories,
        List<String> dependencies,
        List<String> launcherDependencies,
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline,
        String reportsDir,
        List<String> includeTags,
        List<String> excludeTags,
        List<String> includeEngines,
        List<String> excludeEngines,
        boolean failIfNoTests
    ) {
        static TestSpec from(java.util.Map<String, Object> params) {
            val fields = new FieldMap(params);
            return new TestSpec(
                fields.list("classes", ImmutableList.of(JavaDefaults.classesDir("test"))),
                fields.list("classpath", ImmutableList.of()),
                fields.list("repositories", JavaDefaults.REPOSITORIES),
                fields.list("dependencies", ImmutableList.of()),
                fields.list("launcher_dependencies", JavaDefaults.TEST_LAUNCHER_DEPENDENCIES),
                fields.string("local_repository", JavaDefaults.LOCAL_REPOSITORY),
                fields.string("dependency_lock", ""),
                fields.string("dependency_lock_mode", DependencyLock.MODE_OFF),
                fields.bool("offline", false),
                fields.string("reports_dir", JavaDefaults.testResultsDir("test")),
                fields.list("include_tags", ImmutableList.of()),
                fields.list("exclude_tags", ImmutableList.of()),
                fields.list("include_engines", ImmutableList.of()),
                fields.list("exclude_engines", ImmutableList.of()),
                fields.bool("fail_if_no_tests", true)
            );
        }
    }

    private static final class JUnitRuntime {
        private final ClassLoader loader;
        private final Class<?> requestBuilderType;
        private final Class<?> discoverySelectorsType;
        private final Class<?> filterType;
        private final Class<?> launcherFactoryType;
        private final Class<?> launcherType;
        private final Class<?> requestType;
        private final Class<?> testExecutionListenerType;
        private final Class<?> summaryListenerType;
        private final Class<?> summaryType;

        private JUnitRuntime(
            ClassLoader loader,
            Class<?> requestBuilderType,
            Class<?> discoverySelectorsType,
            Class<?> filterType,
            Class<?> launcherFactoryType,
            Class<?> launcherType,
            Class<?> requestType,
            Class<?> testExecutionListenerType,
            Class<?> summaryListenerType,
            Class<?> summaryType
        ) {
            this.loader = loader;
            this.requestBuilderType = requestBuilderType;
            this.discoverySelectorsType = discoverySelectorsType;
            this.filterType = filterType;
            this.launcherFactoryType = launcherFactoryType;
            this.launcherType = launcherType;
            this.requestType = requestType;
            this.testExecutionListenerType = testExecutionListenerType;
            this.summaryListenerType = summaryListenerType;
            this.summaryType = summaryType;
        }

        static JUnitRuntime load(ClassLoader loader) throws Exception {
            return new JUnitRuntime(
                loader,
                type(loader, "org.junit.platform.launcher.core.LauncherDiscoveryRequestBuilder"),
                type(loader, "org.junit.platform.engine.discovery.DiscoverySelectors"),
                type(loader, "org.junit.platform.engine.Filter"),
                type(loader, "org.junit.platform.launcher.core.LauncherFactory"),
                type(loader, "org.junit.platform.launcher.Launcher"),
                type(loader, "org.junit.platform.launcher.LauncherDiscoveryRequest"),
                type(loader, "org.junit.platform.launcher.TestExecutionListener"),
                type(loader, "org.junit.platform.launcher.listeners.SummaryGeneratingListener"),
                type(loader, "org.junit.platform.launcher.listeners.TestExecutionSummary")
            );
        }

        Object requestBuilder() throws Exception {
            return requestBuilderType.getMethod("request").invoke(null);
        }

        void selectClasspathRoots(Object builder, List<Path> classRoots) throws Exception {
            val selectors = discoverySelectorsType
                .getMethod("selectClasspathRoots", java.util.Set.class)
                .invoke(null, new LinkedHashSet<>(classRoots));
            requestBuilderType.getMethod("selectors", java.util.List.class).invoke(builder, selectors);
        }

        Object filter(String typeName, String method, List<String> values) throws Exception {
            return type(loader, typeName).getMethod(method, java.util.List.class).invoke(null, values);
        }

        void filters(Object builder, List<Object> filters) throws Exception {
            if (filters.isEmpty()) {
                return;
            }
            val array = Array.newInstance(filterType, filters.size());
            for (int i = 0; i < filters.size(); i++) {
                Array.set(array, i, filters.get(i));
            }
            requestBuilderType.getMethod("filters", filterType.arrayType()).invoke(builder, array);
        }

        Object buildRequest(Object builder) throws Exception {
            return requestBuilderType.getMethod("build").invoke(builder);
        }

        Object newSummaryListener() throws Exception {
            return summaryListenerType.getConstructor().newInstance();
        }

        void execute(Object request, Object listener) throws Exception {
            val launcher = launcherFactoryType.getMethod("create").invoke(null);
            val array = Array.newInstance(testExecutionListenerType, 1);
            Array.set(array, 0, listener);
            launcherType.getMethod("execute", requestType, testExecutionListenerType.arrayType())
                .invoke(launcher, request, array);
        }

        Object summary(Object listener) throws Exception {
            return summaryListenerType.getMethod("getSummary").invoke(listener);
        }

        long longValue(Object summary, String method) throws Exception {
            return (long) summaryType.getMethod(method).invoke(summary);
        }

        String summaryText(Object summary) throws Exception {
            val out = new StringWriter();
            try (val writer = new PrintWriter(out)) {
                summaryType.getMethod("printTo", PrintWriter.class).invoke(summary, writer);
                if (!((List<?>) summaryType.getMethod("getFailures").invoke(summary)).isEmpty()) {
                    summaryType.getMethod("printFailuresTo", PrintWriter.class).invoke(summary, writer);
                }
            }
            return out.toString();
        }

        private static Class<?> type(ClassLoader loader, String name) throws ClassNotFoundException {
            return Class.forName(name, true, loader);
        }
    }
}
