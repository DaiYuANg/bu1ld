package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import com.google.common.collect.ImmutableMap;
import io.avaje.inject.Component;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import lombok.val;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;
import static org.bu1ld.plugins.java.Protocol.FieldSchema;
import static org.bu1ld.plugins.java.Protocol.Metadata;
import static org.bu1ld.plugins.java.Protocol.PluginConfig;
import static org.bu1ld.plugins.java.Protocol.RuleSchema;
import static org.bu1ld.plugins.java.Protocol.TaskAction;
import static org.bu1ld.plugins.java.Protocol.TaskSpec;

@Component
final class JavaBuildPlugin implements Plugin {
    static final String DEFAULT_ID = "org.bu1ld.java";
    static final String NAMESPACE = "java";
    static final int PROTOCOL_VERSION = 1;

    private static final String ACTION_COMPILE = "compile";
    private static final String ACTION_JAR = "jar";
    private static final String ACTION_RESOURCES = "resources";
    private static final String ACTION_JAVADOC = "javadoc";
    private static final String ACTION_TEST = "test";
    private static final String PLUGIN_EXEC = "plugin.exec";

    private final NativeJavaCompiler compiler;
    private final JarBuilder jarBuilder;
    private final ResourceProcessor resourceProcessor;
    private final JavadocGenerator javadocGenerator;
    private final JUnitTestRunner testRunner;

    JavaBuildPlugin(
        NativeJavaCompiler compiler,
        JarBuilder jarBuilder,
        ResourceProcessor resourceProcessor,
        JavadocGenerator javadocGenerator,
        JUnitTestRunner testRunner
    ) {
        this.compiler = compiler;
        this.jarBuilder = jarBuilder;
        this.resourceProcessor = resourceProcessor;
        this.javadocGenerator = javadocGenerator;
        this.testRunner = testRunner;
    }

    @Override
    public Metadata metadata() {
        return new Metadata(
            DEFAULT_ID,
            NAMESPACE,
            PROTOCOL_VERSION,
            ImmutableList.of("metadata", "expand", "configure", "execute"),
            ImmutableList.of(
                rule("compile",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("srcs", "list", false),
                    field("classpath", "list", false),
                    field("module_path", "list", false),
                    field("repositories", "list", false),
                    field("dependencies", "list", false),
                    field("module_dependencies", "list", false),
                    field("add_modules", "list", false),
                    field("processor_path", "list", false),
                    field("annotation_processors", "list", false),
                    field("processors", "list", false),
                    field("processor_options", "object", false),
                    field("proc", "string", false),
                    field("local_repository", "string", false),
                    field("dependency_lock", "string", false),
                    field("dependency_lock_mode", "string", false),
                    field("offline", "bool", false),
                    field("out", "string", false),
                    field("release", "string", false)
                ),
                rule("resources",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("resources", "list", false),
                    field("resource_roots", "list", false),
                    field("out", "string", false)
                ),
                rule("jar",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("classes", "string", false),
                    field("roots", "list", false),
                    field("out", "string", false)
                ),
                rule("javadoc",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("srcs", "list", false),
                    field("classpath", "list", false),
                    field("module_path", "list", false),
                    field("repositories", "list", false),
                    field("dependencies", "list", false),
                    field("module_dependencies", "list", false),
                    field("add_modules", "list", false),
                    field("local_repository", "string", false),
                    field("dependency_lock", "string", false),
                    field("dependency_lock_mode", "string", false),
                    field("offline", "bool", false),
                    field("out", "string", false),
                    field("release", "string", false)
                ),
                rule("test",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("classes", "list", false),
                    field("classpath", "list", false),
                    field("repositories", "list", false),
                    field("dependencies", "list", false),
                    field("launcher_dependencies", "list", false),
                    field("local_repository", "string", false),
                    field("dependency_lock", "string", false),
                    field("dependency_lock_mode", "string", false),
                    field("offline", "bool", false),
                    field("reports_dir", "string", false),
                    field("include_tags", "list", false),
                    field("exclude_tags", "list", false),
                    field("include_engines", "list", false),
                    field("exclude_engines", "list", false),
                    field("fail_if_no_tests", "bool", false)
                )
            ),
            ImmutableList.of(
                field("name", "string", false),
                field("release", "string", false),
                field("sources", "list", false),
                field("source_roots", "list", false),
                field("resources", "list", false),
                field("resource_roots", "list", false),
                field("classpath", "list", false),
                field("module_path", "list", false),
                field("repositories", "list", false),
                field("dependencies", "list", false),
                field("module_dependencies", "list", false),
                field("add_modules", "list", false),
                field("source_sets", "object", false),
                field("processor_path", "list", false),
                field("annotation_processors", "list", false),
                field("processors", "list", false),
                field("processor_options", "object", false),
                field("proc", "string", false),
                field("test", "bool", false),
                field("reports_dir", "string", false),
                field("include_tags", "list", false),
                field("exclude_tags", "list", false),
                field("include_engines", "list", false),
                field("exclude_engines", "list", false),
                field("fail_if_no_tests", "bool", false),
                field("launcher_dependencies", "list", false),
                field("build_dir", "string", false),
                field("classes_dir", "string", false),
                field("resources_dir", "string", false),
                field("javadoc_dir", "string", false),
                field("local_repository", "string", false),
                field("dependency_lock", "string", false),
                field("dependency_lock_mode", "string", false),
                field("offline", "bool", false),
                field("jar", "string", false),
                field("sources_jar", "string", false),
                field("javadoc_jar", "string", false),
                field("register_build", "bool", false)
            ),
            true
        );
    }

    @Override
    public List<TaskSpec> configure(PluginConfig config) {
        val java = JavaConfig.from(config.fields());
        val main = java.mainSourceSet();

        val tasks = ImmutableList.<TaskSpec>builder();
        val buildDeps = new ArrayList<String>();
        for (val sourceSet : java.sourceSets()) {
            val compileDeps = new ArrayList<String>(sourceSet.compileDeps());
            val classpath = new ArrayList<String>(sourceSet.classpath());
            if (!sourceSet.main()) {
                compileDeps.add(main.classesTask());
                classpath.add(main.classesDir());
                classpath.add(main.resourcesDir());
            }

            tasks.add(compileTask(
                sourceSet.compileTask(),
                compileDeps,
                sourceSet.sources(),
                classpath,
                sourceSet.modulePath(),
                sourceSet.repositories(),
                sourceSet.dependencies(),
                sourceSet.moduleDependencies(),
                sourceSet.addModules(),
                sourceSet.processorPath(),
                sourceSet.annotationProcessors(),
                sourceSet.processors(),
                sourceSet.processorOptions(),
                sourceSet.proc(),
                sourceSet.localRepository(),
                sourceSet.dependencyLock(),
                sourceSet.dependencyLockMode(),
                sourceSet.offline(),
                sourceSet.classesDir(),
                java.release()
            ));
            tasks.add(resourcesTask(
                sourceSet.resourcesTask(),
                ImmutableList.of(),
                sourceSet.resources(),
                sourceSet.resourceRoots(),
                sourceSet.resourcesDir()
            ));
            tasks.add(new TaskSpec(
                sourceSet.classesTask(),
                ImmutableList.of(sourceSet.compileTask(), sourceSet.resourcesTask()),
                ImmutableList.of(sourceSet.classesDir() + "/**/*", sourceSet.resourcesDir() + "/**/*"),
                ImmutableList.of(sourceSet.classesDir(), sourceSet.resourcesDir()),
                null,
                null
            ));
            if (!sourceSet.main()) {
                buildDeps.add(sourceSet.classesTask());
            }
            if (sourceSet.test()) {
                tasks.add(testTask(
                    sourceSet.testTask(),
                    ImmutableList.of(sourceSet.classesTask()),
                    ImmutableList.of(sourceSet.classesDir()),
                    runtimeClasspath(sourceSet, main),
                    sourceSet.repositories(),
                    sourceSet.dependencies(),
                    sourceSet.launcherDependencies(),
                    sourceSet.localRepository(),
                    sourceSet.dependencyLock(),
                    sourceSet.dependencyLockMode(),
                    sourceSet.offline(),
                    sourceSet.reportsDir(),
                    sourceSet.includeTags(),
                    sourceSet.excludeTags(),
                    sourceSet.includeEngines(),
                    sourceSet.excludeEngines(),
                    sourceSet.failIfNoTests()
                ));
                buildDeps.remove(sourceSet.classesTask());
                buildDeps.add(sourceSet.testTask());
            }
        }
        tasks.add(jarTask("jar", ImmutableList.of(main.classesTask()), ImmutableList.of(main.classesDir(), main.resourcesDir()), java.jar()));
        tasks.add(javadocTask(
            "javadoc",
            ImmutableList.of(main.compileTask()),
            main.sources(),
            combine(main.classpath(), ImmutableList.of(main.classesDir())),
            main.modulePath(),
            main.repositories(),
            main.dependencies(),
            main.moduleDependencies(),
            main.addModules(),
            main.localRepository(),
            main.dependencyLock(),
            main.dependencyLockMode(),
            main.offline(),
            java.javadocDir(),
            java.release()
        ));
        tasks.add(jarTask("sourcesJar", ImmutableList.of(), combine(main.sourceRoots(), main.resourceRoots()), java.sourcesJar()));
        tasks.add(jarTask("javadocJar", ImmutableList.of("javadoc"), ImmutableList.of(java.javadocDir()), java.javadocJar()));
        if (java.registerBuild()) {
            buildDeps.addAll(ImmutableList.of("jar", "sourcesJar", "javadocJar"));
            tasks.add(new TaskSpec(
                "build",
                ImmutableList.copyOf(buildDeps),
                ImmutableList.of(),
                ImmutableList.of(),
                null,
                null
            ));
        }
        return tasks.build();
    }

    @Override
    public List<TaskSpec> expand(Invocation invocation) {
        return switch (invocation.rule()) {
            case "compile" -> ImmutableList.of(expandCompile(invocation));
            case "resources" -> ImmutableList.of(expandResources(invocation));
            case "jar" -> ImmutableList.of(expandJar(invocation));
            case "javadoc" -> ImmutableList.of(expandJavadoc(invocation));
            case "test" -> ImmutableList.of(expandTest(invocation));
            default -> ImmutableList.of();
        };
    }

    @Override
    public ExecuteResult execute(ExecuteRequest request) throws Exception {
        return switch (request.action()) {
            case ACTION_COMPILE -> compiler.compile(request);
            case ACTION_RESOURCES -> resourceProcessor.process(request);
            case ACTION_JAR -> jarBuilder.create(request);
            case ACTION_JAVADOC -> javadocGenerator.generate(request);
            case ACTION_TEST -> testRunner.run(request);
            default -> throw new IllegalArgumentException("unknown java action \"" + request.action() + "\"");
        };
    }

    private TaskSpec expandCompile(Invocation invocation) {
        val srcs = invocation.optionalList("srcs", JavaDefaults.SOURCES);
        val classpath = invocation.optionalList("classpath", ImmutableList.of());
        val modulePath = invocation.optionalList("module_path", ImmutableList.of());
        val repositories = invocation.optionalList("repositories", JavaDefaults.REPOSITORIES);
        val dependencies = invocation.optionalList("dependencies", ImmutableList.of());
        val moduleDependencies = invocation.optionalList("module_dependencies", ImmutableList.of());
        val addModules = invocation.optionalList("add_modules", ImmutableList.of());
        val processorPath = invocation.optionalList("processor_path", ImmutableList.of());
        val annotationProcessors = invocation.optionalList("annotation_processors", ImmutableList.of());
        val processors = invocation.optionalList("processors", ImmutableList.of());
        val processorOptions = new FieldMap(invocation.fields()).stringMap("processor_options", Map.of());
        val proc = invocation.optionalString("proc", "");
        val localRepository = invocation.optionalString("local_repository", JavaDefaults.LOCAL_REPOSITORY);
        val dependencyLock = invocation.optionalString("dependency_lock", "");
        val dependencyLockMode = invocation.optionalString("dependency_lock_mode", DependencyLock.MODE_OFF);
        val offline = invocation.optionalBool("offline", false);
        val out = invocation.optionalString("out", JavaDefaults.CLASSES_DIR);
        val release = invocation.optionalString("release", JavaDefaults.RELEASE);
        return compileTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", compileInputs(srcs, classpath, modulePath, processorPath, dependencyLock)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            srcs,
            classpath,
            modulePath,
            repositories,
            dependencies,
            moduleDependencies,
            addModules,
            processorPath,
            annotationProcessors,
            processors,
            processorOptions,
            proc,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            out,
            release
        );
    }

    private TaskSpec expandResources(Invocation invocation) {
        val resources = invocation.optionalList("resources", JavaDefaults.RESOURCES);
        val resourceRoots = invocation.optionalList("resource_roots", JavaDefaults.RESOURCE_ROOTS);
        val out = invocation.optionalString("out", JavaDefaults.RESOURCES_DIR);
        return resourcesTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", resources),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            resources,
            resourceRoots,
            out
        );
    }

    private TaskSpec expandJar(Invocation invocation) {
        val classes = invocation.optionalString("classes", JavaDefaults.CLASSES_DIR);
        val roots = invocation.optionalList("roots", ImmutableList.of(classes));
        val out = invocation.optionalString("out", JavaDefaults.jar("app"));
        return jarTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", jarInputs(roots)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            roots,
            out
        );
    }

    private TaskSpec expandJavadoc(Invocation invocation) {
        val srcs = invocation.optionalList("srcs", JavaDefaults.SOURCES);
        val classpath = invocation.optionalList("classpath", ImmutableList.of());
        val modulePath = invocation.optionalList("module_path", ImmutableList.of());
        val repositories = invocation.optionalList("repositories", JavaDefaults.REPOSITORIES);
        val dependencies = invocation.optionalList("dependencies", ImmutableList.of());
        val moduleDependencies = invocation.optionalList("module_dependencies", ImmutableList.of());
        val addModules = invocation.optionalList("add_modules", ImmutableList.of());
        val localRepository = invocation.optionalString("local_repository", JavaDefaults.LOCAL_REPOSITORY);
        val dependencyLock = invocation.optionalString("dependency_lock", "");
        val dependencyLockMode = invocation.optionalString("dependency_lock_mode", DependencyLock.MODE_OFF);
        val offline = invocation.optionalBool("offline", false);
        val out = invocation.optionalString("out", JavaDefaults.JAVADOC_DIR);
        val release = invocation.optionalString("release", JavaDefaults.RELEASE);
        return javadocTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", compileInputs(srcs, classpath, modulePath, ImmutableList.of(), dependencyLock)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            srcs,
            classpath,
            modulePath,
            repositories,
            dependencies,
            moduleDependencies,
            addModules,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            out,
            release
        );
    }

    private TaskSpec expandTest(Invocation invocation) {
        val classes = invocation.optionalList("classes", ImmutableList.of(JavaDefaults.classesDir("test")));
        val classpath = invocation.optionalList("classpath", ImmutableList.of());
        val repositories = invocation.optionalList("repositories", JavaDefaults.REPOSITORIES);
        val dependencies = invocation.optionalList("dependencies", ImmutableList.of());
        val launcherDependencies = invocation.optionalList("launcher_dependencies", JavaDefaults.TEST_LAUNCHER_DEPENDENCIES);
        val localRepository = invocation.optionalString("local_repository", JavaDefaults.LOCAL_REPOSITORY);
        val dependencyLock = invocation.optionalString("dependency_lock", "");
        val dependencyLockMode = invocation.optionalString("dependency_lock_mode", DependencyLock.MODE_OFF);
        val offline = invocation.optionalBool("offline", false);
        val reportsDir = invocation.optionalString("reports_dir", JavaDefaults.testResultsDir("test"));
        val includeTags = invocation.optionalList("include_tags", ImmutableList.of());
        val excludeTags = invocation.optionalList("exclude_tags", ImmutableList.of());
        val includeEngines = invocation.optionalList("include_engines", ImmutableList.of());
        val excludeEngines = invocation.optionalList("exclude_engines", ImmutableList.of());
        val failIfNoTests = invocation.optionalBool("fail_if_no_tests", true);
        return testTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", testInputs(classes, classpath, dependencyLock)),
            invocation.optionalList("outputs", ImmutableList.of(reportsDir)),
            classes,
            classpath,
            repositories,
            dependencies,
            launcherDependencies,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            reportsDir,
            includeTags,
            excludeTags,
            includeEngines,
            excludeEngines,
            failIfNoTests
        );
    }

    private TaskSpec compileTask(
        String name,
        List<String> deps,
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
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline,
        String out,
        String release
    ) {
        return compileTask(
            name,
            deps,
            compileInputs(srcs, classpath, modulePath, processorPath, dependencyLock),
            ImmutableList.of(out),
            srcs,
            classpath,
            modulePath,
            repositories,
            dependencies,
            moduleDependencies,
            addModules,
            processorPath,
            annotationProcessors,
            processors,
            processorOptions,
            proc,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            out,
            release
        );
    }

    private TaskSpec compileTask(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
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
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline,
        String out,
        String release
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_COMPILE, ImmutableMap.<String, Object>builder()
            .put("srcs", srcs)
            .put("classpath", classpath)
            .put("module_path", modulePath)
            .put("repositories", repositories)
            .put("dependencies", dependencies)
            .put("module_dependencies", moduleDependencies)
            .put("add_modules", addModules)
            .put("processor_path", processorPath)
            .put("annotation_processors", annotationProcessors)
            .put("processors", processors)
            .put("processor_options", processorOptions)
            .put("proc", proc)
            .put("local_repository", localRepository)
            .put("dependency_lock", dependencyLock)
            .put("dependency_lock_mode", dependencyLockMode)
            .put("offline", offline)
            .put("out", out)
            .put("release", release)
            .build()));
    }

    private TaskSpec resourcesTask(String name, List<String> deps, List<String> resources, List<String> resourceRoots, String out) {
        return resourcesTask(name, deps, resources, ImmutableList.of(out), resources, resourceRoots, out);
    }

    private TaskSpec resourcesTask(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
        List<String> resources,
        List<String> resourceRoots,
        String out
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_RESOURCES, ImmutableMap.of(
            "resources", resources,
            "resource_roots", resourceRoots,
            "out", out
        )));
    }

    private TaskSpec jarTask(String name, List<String> deps, List<String> roots, String out) {
        return jarTask(name, deps, jarInputs(roots), ImmutableList.of(out), roots, out);
    }

    private TaskSpec jarTask(String name, List<String> deps, List<String> inputs, List<String> outputs, List<String> roots, String out) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_JAR, ImmutableMap.of(
            "roots", roots,
            "out", out
        )));
    }

    private TaskSpec javadocTask(
        String name,
        List<String> deps,
        List<String> srcs,
        List<String> classpath,
        List<String> modulePath,
        List<String> repositories,
        List<String> dependencies,
        List<String> moduleDependencies,
        List<String> addModules,
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline,
        String out,
        String release
    ) {
        return javadocTask(
            name,
            deps,
            compileInputs(srcs, classpath, modulePath, ImmutableList.of(), dependencyLock),
            ImmutableList.of(out),
            srcs,
            classpath,
            modulePath,
            repositories,
            dependencies,
            moduleDependencies,
            addModules,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            out,
            release
        );
    }

    private TaskSpec javadocTask(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
        List<String> srcs,
        List<String> classpath,
        List<String> modulePath,
        List<String> repositories,
        List<String> dependencies,
        List<String> moduleDependencies,
        List<String> addModules,
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline,
        String out,
        String release
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_JAVADOC, ImmutableMap.<String, Object>builder()
            .put("srcs", srcs)
            .put("classpath", classpath)
            .put("module_path", modulePath)
            .put("repositories", repositories)
            .put("dependencies", dependencies)
            .put("module_dependencies", moduleDependencies)
            .put("add_modules", addModules)
            .put("local_repository", localRepository)
            .put("dependency_lock", dependencyLock)
            .put("dependency_lock_mode", dependencyLockMode)
            .put("offline", offline)
            .put("out", out)
            .put("release", release)
            .build()));
    }

    private TaskSpec testTask(
        String name,
        List<String> deps,
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
        return testTask(
            name,
            deps,
            testInputs(classes, classpath, dependencyLock),
            ImmutableList.of(reportsDir),
            classes,
            classpath,
            repositories,
            dependencies,
            launcherDependencies,
            localRepository,
            dependencyLock,
            dependencyLockMode,
            offline,
            reportsDir,
            includeTags,
            excludeTags,
            includeEngines,
            excludeEngines,
            failIfNoTests
        );
    }

    private TaskSpec testTask(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
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
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_TEST, ImmutableMap.<String, Object>builder()
            .put("classes", classes)
            .put("classpath", classpath)
            .put("repositories", repositories)
            .put("dependencies", dependencies)
            .put("launcher_dependencies", launcherDependencies)
            .put("local_repository", localRepository)
            .put("dependency_lock", dependencyLock)
            .put("dependency_lock_mode", dependencyLockMode)
            .put("offline", offline)
            .put("reports_dir", reportsDir)
            .put("include_tags", includeTags)
            .put("exclude_tags", excludeTags)
            .put("include_engines", includeEngines)
            .put("exclude_engines", excludeEngines)
            .put("fail_if_no_tests", failIfNoTests)
            .build()));
    }

    private TaskAction pluginAction(String action, Map<String, Object> params) {
        val actionParams = new LinkedHashMap<String, Object>();
        actionParams.put("namespace", NAMESPACE);
        actionParams.put("action", action);
        actionParams.put("params", params);
        return new TaskAction(PLUGIN_EXEC, actionParams);
    }

    private List<String> compileInputs(
        List<String> srcs,
        List<String> classpath,
        List<String> modulePath,
        List<String> processorPath,
        String dependencyLock
    ) {
        val inputs = new ArrayList<String>(srcs.size() + classpath.size() + modulePath.size() + processorPath.size() + 1);
        inputs.addAll(srcs);
        inputs.addAll(classpath);
        inputs.addAll(modulePath);
        inputs.addAll(processorPath);
        if (!dependencyLock.isBlank()) {
            inputs.add(dependencyLock);
        }
        return ImmutableList.copyOf(inputs);
    }

    private List<String> testInputs(List<String> classes, List<String> classpath, String dependencyLock) {
        val inputs = new ArrayList<String>(classes.size() + classpath.size() + 1);
        for (val item : classes) {
            inputs.add(item + "/**/*");
        }
        inputs.addAll(classpath);
        if (!dependencyLock.isBlank()) {
            inputs.add(dependencyLock);
        }
        return ImmutableList.copyOf(inputs);
    }

    private List<String> jarInputs(List<String> roots) {
        val inputs = new ArrayList<String>(roots.size());
        for (val root : roots) {
            inputs.add(root + "/**/*");
        }
        return ImmutableList.copyOf(inputs);
    }

    private List<String> combine(List<String> first, List<String> second) {
        val result = new ArrayList<String>(first.size() + second.size());
        result.addAll(first);
        result.addAll(second);
        return ImmutableList.copyOf(result);
    }

    private List<String> runtimeClasspath(SourceSetConfig sourceSet, SourceSetConfig main) {
        val classpath = new ArrayList<String>(sourceSet.classpath().size() + 4);
        classpath.add(sourceSet.classesDir());
        classpath.add(sourceSet.resourcesDir());
        if (!sourceSet.main()) {
            classpath.add(main.classesDir());
            classpath.add(main.resourcesDir());
        }
        classpath.addAll(sourceSet.classpath());
        return ImmutableList.copyOf(classpath);
    }

    private static RuleSchema rule(String name, FieldSchema... fields) {
        return new RuleSchema(name, ImmutableList.copyOf(fields));
    }

    private static FieldSchema field(String name, String type, boolean required) {
        return new FieldSchema(name, type, required);
    }

    private record JavaConfig(
        String name,
        String release,
        List<SourceSetConfig> sourceSets,
        String javadocDir,
        String jar,
        String sourcesJar,
        String javadocJar,
        boolean registerBuild
    ) {
        SourceSetConfig mainSourceSet() {
            for (val sourceSet : sourceSets) {
                if (sourceSet.main()) {
                    return sourceSet;
                }
            }
            return sourceSets.get(0);
        }

        static JavaConfig from(Map<String, Object> fields) {
            val values = new FieldMap(fields);
            val name = values.string("name", "app");
            val buildDir = values.string("build_dir", JavaDefaults.BUILD_DIR);
            val javadocDir = values.string("javadoc_dir", buildDir + "/docs/javadoc");
            return new JavaConfig(
                name,
                values.string("release", JavaDefaults.RELEASE),
                sourceSets(values, buildDir),
                javadocDir,
                values.string("jar", buildDir + "/libs/" + name + ".jar"),
                values.string("sources_jar", buildDir + "/libs/" + name + "-sources.jar"),
                values.string("javadoc_jar", buildDir + "/libs/" + name + "-javadoc.jar"),
                values.bool("register_build", true)
            );
        }

        private static List<SourceSetConfig> sourceSets(FieldMap values, String buildDir) {
            val configured = values.object("source_sets", Map.of());
            val names = new ArrayList<String>(configured.keySet());
            names.remove("main");
            names.sort(String::compareTo);
            names.add(0, "main");

            val sourceSets = ImmutableList.<SourceSetConfig>builder();
            for (val name : names) {
                sourceSets.add(SourceSetConfig.from(name, values, new FieldMap(sourceSetFields(configured, name)), buildDir));
            }
            return sourceSets.build();
        }

        private static Map<String, Object> sourceSetFields(Map<String, Object> configured, String name) {
            if (!configured.containsKey(name)) {
                return Map.of();
            }
            val value = configured.get(name);
            if (value instanceof Map<?, ?> map) {
                val result = new LinkedHashMap<String, Object>(map.size());
                for (val entry : map.entrySet()) {
                    if (entry.getKey() instanceof String key) {
                        result.put(key, entry.getValue());
                        continue;
                    }
                    throw new IllegalArgumentException("source_sets." + name + " must be object");
                }
                return result;
            }
            throw new IllegalArgumentException("source_sets." + name + " must be object");
        }
    }

    private record SourceSetConfig(
        String name,
        List<String> sources,
        List<String> sourceRoots,
        List<String> resources,
        List<String> resourceRoots,
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
        List<String> launcherDependencies,
        List<String> compileDeps,
        boolean test,
        String reportsDir,
        List<String> includeTags,
        List<String> excludeTags,
        List<String> includeEngines,
        List<String> excludeEngines,
        boolean failIfNoTests,
        String classesDir,
        String resourcesDir,
        String localRepository,
        String dependencyLock,
        String dependencyLockMode,
        boolean offline
    ) {
        static SourceSetConfig from(String name, FieldMap global, FieldMap local, String buildDir) {
            val main = "main".equals(name);
            val defaultSources = main ? JavaDefaults.SOURCES : JavaDefaults.sources(name);
            val defaultSourceRoots = main ? JavaDefaults.SOURCE_ROOTS : JavaDefaults.sourceRoots(name);
            val defaultResources = main ? JavaDefaults.RESOURCES : JavaDefaults.resources(name);
            val defaultResourceRoots = main ? JavaDefaults.RESOURCE_ROOTS : JavaDefaults.resourceRoots(name);
            val defaultClassesDir = main
                ? global.string("classes_dir", buildDir + "/classes/java/main")
                : buildDir + "/classes/java/" + name;
            val defaultResourcesDir = main
                ? global.string("resources_dir", buildDir + "/resources/main")
                : buildDir + "/resources/" + name;

            return new SourceSetConfig(
                name,
                list(global, local, "sources", main ? global.list("sources", defaultSources) : defaultSources),
                list(global, local, "source_roots", main ? global.list("source_roots", defaultSourceRoots) : defaultSourceRoots),
                list(global, local, "resources", main ? global.list("resources", defaultResources) : defaultResources),
                list(global, local, "resource_roots", main ? global.list("resource_roots", defaultResourceRoots) : defaultResourceRoots),
                mergedList(global, local, "classpath"),
                mergedList(global, local, "module_path"),
                local.has("repositories") ? local.list("repositories", JavaDefaults.REPOSITORIES) : global.list("repositories", JavaDefaults.REPOSITORIES),
                mergedList(global, local, "dependencies"),
                mergedList(global, local, "module_dependencies"),
                mergedList(global, local, "add_modules"),
                mergedList(global, local, "processor_path"),
                mergedList(global, local, "annotation_processors"),
                mergedList(global, local, "processors"),
                mergedStringMap(global, local, "processor_options"),
                string(global, local, "proc", ""),
                list(global, local, "launcher_dependencies", JavaDefaults.TEST_LAUNCHER_DEPENDENCIES),
                local.list("compile_deps", ImmutableList.of()),
                bool(global, local, "test", "test".equals(name)),
                local.string("reports_dir", JavaDefaults.testResultsDir(name)),
                list(global, local, "include_tags", ImmutableList.of()),
                list(global, local, "exclude_tags", ImmutableList.of()),
                list(global, local, "include_engines", ImmutableList.of()),
                list(global, local, "exclude_engines", ImmutableList.of()),
                bool(global, local, "fail_if_no_tests", true),
                local.string("classes_dir", defaultClassesDir),
                local.string("resources_dir", defaultResourcesDir),
                string(global, local, "local_repository", JavaDefaults.LOCAL_REPOSITORY),
                string(global, local, "dependency_lock", ""),
                string(global, local, "dependency_lock_mode", DependencyLock.MODE_OFF),
                bool(global, local, "offline", false)
            );
        }

        boolean main() {
            return "main".equals(name);
        }

        String compileTask() {
            if (main()) {
                return "compileJava";
            }
            return "compile" + capitalizedName() + "Java";
        }

        String resourcesTask() {
            if (main()) {
                return "processResources";
            }
            return "process" + capitalizedName() + "Resources";
        }

        String classesTask() {
            if (main()) {
                return "classes";
            }
            return name + "Classes";
        }

        String testTask() {
            if ("test".equals(name)) {
                return "test";
            }
            return name;
        }

        private String capitalizedName() {
            if (name.isBlank()) {
                return name;
            }
            return name.substring(0, 1).toUpperCase() + name.substring(1);
        }

        private static List<String> list(FieldMap global, FieldMap local, String name, List<String> fallback) {
            if (local.has(name)) {
                return local.list(name, fallback);
            }
            return fallback;
        }

        private static String string(FieldMap global, FieldMap local, String name, String fallback) {
            if (local.has(name)) {
                return local.string(name, fallback);
            }
            return global.string(name, fallback);
        }

        private static boolean bool(FieldMap global, FieldMap local, String name, boolean fallback) {
            if (local.has(name)) {
                return local.bool(name, fallback);
            }
            return global.bool(name, fallback);
        }

        private static List<String> mergedList(FieldMap global, FieldMap local, String name) {
            val values = new ArrayList<String>();
            values.addAll(global.list(name, ImmutableList.of()));
            if (local.has(name)) {
                values.addAll(local.list(name, ImmutableList.of()));
            }
            return ImmutableList.copyOf(values);
        }

        private static Map<String, String> mergedStringMap(FieldMap global, FieldMap local, String name) {
            val values = new LinkedHashMap<String, String>();
            values.putAll(global.stringMap(name, Map.of()));
            if (local.has(name)) {
                values.putAll(local.stringMap(name, Map.of()));
            }
            return ImmutableMap.copyOf(values);
        }
    }
}
