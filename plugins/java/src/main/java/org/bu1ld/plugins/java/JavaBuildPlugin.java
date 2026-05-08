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
    private static final String PLUGIN_EXEC = "plugin.exec";

    private final NativeJavaCompiler compiler;
    private final JarBuilder jarBuilder;
    private final ResourceProcessor resourceProcessor;
    private final JavadocGenerator javadocGenerator;

    JavaBuildPlugin(
        NativeJavaCompiler compiler,
        JarBuilder jarBuilder,
        ResourceProcessor resourceProcessor,
        JavadocGenerator javadocGenerator
    ) {
        this.compiler = compiler;
        this.jarBuilder = jarBuilder;
        this.resourceProcessor = resourceProcessor;
        this.javadocGenerator = javadocGenerator;
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
                    field("repositories", "list", false),
                    field("dependencies", "list", false),
                    field("local_repository", "string", false),
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
                    field("repositories", "list", false),
                    field("dependencies", "list", false),
                    field("local_repository", "string", false),
                    field("out", "string", false),
                    field("release", "string", false)
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
                field("repositories", "list", false),
                field("dependencies", "list", false),
                field("build_dir", "string", false),
                field("classes_dir", "string", false),
                field("resources_dir", "string", false),
                field("javadoc_dir", "string", false),
                field("local_repository", "string", false),
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

        val tasks = ImmutableList.<TaskSpec>builder();
        tasks.add(compileTask(
            "compileJava",
            ImmutableList.of(),
            java.sources(),
            java.classpath(),
            java.repositories(),
            java.dependencies(),
            java.localRepository(),
            java.classesDir(),
            java.release()
        ));
        tasks.add(resourcesTask(
            "processResources",
            ImmutableList.of(),
            java.resources(),
            java.resourceRoots(),
            java.resourcesDir()
        ));
        tasks.add(new TaskSpec(
            "classes",
            ImmutableList.of("compileJava", "processResources"),
            ImmutableList.of(java.classesDir() + "/**/*", java.resourcesDir() + "/**/*"),
            ImmutableList.of(java.classesDir(), java.resourcesDir()),
            null,
            null
        ));
        tasks.add(jarTask("jar", ImmutableList.of("classes"), ImmutableList.of(java.classesDir(), java.resourcesDir()), java.jar()));
        tasks.add(javadocTask(
            "javadoc",
            ImmutableList.of("compileJava"),
            java.sources(),
            java.classpath(),
            java.repositories(),
            java.dependencies(),
            java.localRepository(),
            java.javadocDir(),
            java.release()
        ));
        tasks.add(jarTask("sourcesJar", ImmutableList.of(), combine(java.sourceRoots(), java.resourceRoots()), java.sourcesJar()));
        tasks.add(jarTask("javadocJar", ImmutableList.of("javadoc"), ImmutableList.of(java.javadocDir()), java.javadocJar()));
        if (java.registerBuild()) {
            tasks.add(new TaskSpec(
                "build",
                ImmutableList.of("jar", "sourcesJar", "javadocJar"),
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
            default -> throw new IllegalArgumentException("unknown java action \"" + request.action() + "\"");
        };
    }

    private TaskSpec expandCompile(Invocation invocation) {
        val srcs = invocation.optionalList("srcs", JavaDefaults.SOURCES);
        val classpath = invocation.optionalList("classpath", ImmutableList.of());
        val repositories = invocation.optionalList("repositories", JavaDefaults.REPOSITORIES);
        val dependencies = invocation.optionalList("dependencies", ImmutableList.of());
        val localRepository = invocation.optionalString("local_repository", JavaDefaults.LOCAL_REPOSITORY);
        val out = invocation.optionalString("out", JavaDefaults.CLASSES_DIR);
        val release = invocation.optionalString("release", JavaDefaults.RELEASE);
        return compileTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", compileInputs(srcs, classpath)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            srcs,
            classpath,
            repositories,
            dependencies,
            localRepository,
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
        val repositories = invocation.optionalList("repositories", JavaDefaults.REPOSITORIES);
        val dependencies = invocation.optionalList("dependencies", ImmutableList.of());
        val localRepository = invocation.optionalString("local_repository", JavaDefaults.LOCAL_REPOSITORY);
        val out = invocation.optionalString("out", JavaDefaults.JAVADOC_DIR);
        val release = invocation.optionalString("release", JavaDefaults.RELEASE);
        return javadocTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", compileInputs(srcs, classpath)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            srcs,
            classpath,
            repositories,
            dependencies,
            localRepository,
            out,
            release
        );
    }

    private TaskSpec compileTask(
        String name,
        List<String> deps,
        List<String> srcs,
        List<String> classpath,
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        String out,
        String release
    ) {
        return compileTask(
            name,
            deps,
            compileInputs(srcs, classpath),
            ImmutableList.of(out),
            srcs,
            classpath,
            repositories,
            dependencies,
            localRepository,
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
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        String out,
        String release
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_COMPILE, ImmutableMap.<String, Object>builder()
            .put("srcs", srcs)
            .put("classpath", classpath)
            .put("repositories", repositories)
            .put("dependencies", dependencies)
            .put("local_repository", localRepository)
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
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        String out,
        String release
    ) {
        return javadocTask(
            name,
            deps,
            compileInputs(srcs, classpath),
            ImmutableList.of(out),
            srcs,
            classpath,
            repositories,
            dependencies,
            localRepository,
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
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        String out,
        String release
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_JAVADOC, ImmutableMap.<String, Object>builder()
            .put("srcs", srcs)
            .put("classpath", classpath)
            .put("repositories", repositories)
            .put("dependencies", dependencies)
            .put("local_repository", localRepository)
            .put("out", out)
            .put("release", release)
            .build()));
    }

    private TaskAction pluginAction(String action, Map<String, Object> params) {
        val actionParams = new LinkedHashMap<String, Object>();
        actionParams.put("namespace", NAMESPACE);
        actionParams.put("action", action);
        actionParams.put("params", params);
        return new TaskAction(PLUGIN_EXEC, actionParams);
    }

    private List<String> compileInputs(List<String> srcs, List<String> classpath) {
        val inputs = new ArrayList<String>(srcs);
        inputs.addAll(classpath);
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

    private static RuleSchema rule(String name, FieldSchema... fields) {
        return new RuleSchema(name, ImmutableList.copyOf(fields));
    }

    private static FieldSchema field(String name, String type, boolean required) {
        return new FieldSchema(name, type, required);
    }

    private record JavaConfig(
        String name,
        String release,
        List<String> sources,
        List<String> sourceRoots,
        List<String> resources,
        List<String> resourceRoots,
        List<String> classpath,
        List<String> repositories,
        List<String> dependencies,
        String classesDir,
        String resourcesDir,
        String javadocDir,
        String localRepository,
        String jar,
        String sourcesJar,
        String javadocJar,
        boolean registerBuild
    ) {
        static JavaConfig from(Map<String, Object> fields) {
            val values = new FieldMap(fields);
            val name = values.string("name", "app");
            val buildDir = values.string("build_dir", JavaDefaults.BUILD_DIR);
            val classesDir = values.string("classes_dir", buildDir + "/classes/java/main");
            val resourcesDir = values.string("resources_dir", buildDir + "/resources/main");
            val javadocDir = values.string("javadoc_dir", buildDir + "/docs/javadoc");
            return new JavaConfig(
                name,
                values.string("release", JavaDefaults.RELEASE),
                values.list("sources", JavaDefaults.SOURCES),
                values.list("source_roots", JavaDefaults.SOURCE_ROOTS),
                values.list("resources", JavaDefaults.RESOURCES),
                values.list("resource_roots", JavaDefaults.RESOURCE_ROOTS),
                values.list("classpath", ImmutableList.of()),
                values.list("repositories", JavaDefaults.REPOSITORIES),
                values.list("dependencies", ImmutableList.of()),
                classesDir,
                resourcesDir,
                javadocDir,
                values.string("local_repository", JavaDefaults.LOCAL_REPOSITORY),
                values.string("jar", buildDir + "/libs/" + name + ".jar"),
                values.string("sources_jar", buildDir + "/libs/" + name + "-sources.jar"),
                values.string("javadoc_jar", buildDir + "/libs/" + name + "-javadoc.jar"),
                values.bool("register_build", true)
            );
        }
    }
}
