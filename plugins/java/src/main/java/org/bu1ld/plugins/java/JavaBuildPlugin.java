package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import com.google.common.collect.ImmutableMap;
import io.avaje.inject.Component;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

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

    private static final String ACTION_COMPILE = "compile";
    private static final String ACTION_JAR = "jar";
    private static final String PLUGIN_EXEC = "plugin.exec";

    private final NativeJavaCompiler compiler;
    private final JarBuilder jarBuilder;

    JavaBuildPlugin(NativeJavaCompiler compiler, JarBuilder jarBuilder) {
        this.compiler = compiler;
        this.jarBuilder = jarBuilder;
    }

    @Override
    public Metadata metadata() {
        return new Metadata(
            DEFAULT_ID,
            NAMESPACE,
            ImmutableList.of(
                rule("compile",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("srcs", "list", false),
                    field("classpath", "list", false),
                    field("out", "string", false),
                    field("release", "string", false)
                ),
                rule("jar",
                    field("deps", "list", false),
                    field("inputs", "list", false),
                    field("outputs", "list", false),
                    field("classes", "string", false),
                    field("out", "string", false)
                )
            ),
            ImmutableList.of(
                field("name", "string", false),
                field("release", "string", false),
                field("srcs", "list", false),
                field("classpath", "list", false),
                field("build_dir", "string", false),
                field("classes_dir", "string", false),
                field("jar", "string", false),
                field("register_build", "bool", false)
            ),
            true
        );
    }

    @Override
    public List<TaskSpec> configure(PluginConfig config) {
        JavaConfig java = JavaConfig.from(config.fields());

        ImmutableList.Builder<TaskSpec> tasks = ImmutableList.builder();
        tasks.add(compileTask("compileJava", ImmutableList.of(), java.srcs(), java.classpath(), java.classesDir(), java.release()));
        tasks.add(new TaskSpec(
            "classes",
            ImmutableList.of("compileJava"),
            ImmutableList.of(java.classesDir() + "/**/*"),
            ImmutableList.of(java.classesDir()),
            null,
            null
        ));
        tasks.add(jarTask("jar", ImmutableList.of("classes"), java.classesDir(), java.jar()));
        if (java.registerBuild()) {
            tasks.add(new TaskSpec("build", ImmutableList.of("jar"), ImmutableList.of(), ImmutableList.of(), null, null));
        }
        return tasks.build();
    }

    @Override
    public List<TaskSpec> expand(Invocation invocation) {
        return switch (invocation.rule()) {
            case "compile" -> ImmutableList.of(expandCompile(invocation));
            case "jar" -> ImmutableList.of(expandJar(invocation));
            default -> ImmutableList.of();
        };
    }

    @Override
    public ExecuteResult execute(ExecuteRequest request) throws Exception {
        return switch (request.action()) {
            case ACTION_COMPILE -> compiler.compile(request);
            case ACTION_JAR -> jarBuilder.create(request);
            default -> throw new IllegalArgumentException("unknown java action \"" + request.action() + "\"");
        };
    }

    private TaskSpec expandCompile(Invocation invocation) {
        List<String> srcs = invocation.optionalList("srcs", JavaConfig.DEFAULT_SRCS);
        List<String> classpath = invocation.optionalList("classpath", ImmutableList.of());
        String out = invocation.optionalString("out", JavaConfig.DEFAULT_CLASSES_DIR);
        String release = invocation.optionalString("release", JavaConfig.DEFAULT_RELEASE);
        return compileTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", compileInputs(srcs, classpath)),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            srcs,
            classpath,
            out,
            release
        );
    }

    private TaskSpec expandJar(Invocation invocation) {
        String classes = invocation.optionalString("classes", JavaConfig.DEFAULT_CLASSES_DIR);
        String out = invocation.optionalString("out", JavaConfig.DEFAULT_JAR);
        return jarTask(
            invocation.target(),
            invocation.optionalList("deps", ImmutableList.of()),
            invocation.optionalList("inputs", ImmutableList.of(classes + "/**/*")),
            invocation.optionalList("outputs", ImmutableList.of(out)),
            classes,
            out
        );
    }

    private TaskSpec compileTask(String name, List<String> deps, List<String> srcs, List<String> classpath, String out, String release) {
        return compileTask(name, deps, compileInputs(srcs, classpath), ImmutableList.of(out), srcs, classpath, out, release);
    }

    private TaskSpec compileTask(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
        List<String> srcs,
        List<String> classpath,
        String out,
        String release
    ) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_COMPILE, ImmutableMap.of(
            "srcs", srcs,
            "classpath", classpath,
            "out", out,
            "release", release
        )));
    }

    private TaskSpec jarTask(String name, List<String> deps, String classes, String out) {
        return jarTask(name, deps, ImmutableList.of(classes + "/**/*"), ImmutableList.of(out), classes, out);
    }

    private TaskSpec jarTask(String name, List<String> deps, List<String> inputs, List<String> outputs, String classes, String out) {
        return new TaskSpec(name, deps, inputs, outputs, null, pluginAction(ACTION_JAR, ImmutableMap.of(
            "classes", classes,
            "out", out
        )));
    }

    private TaskAction pluginAction(String action, Map<String, Object> params) {
        Map<String, Object> actionParams = new LinkedHashMap<>();
        actionParams.put("namespace", NAMESPACE);
        actionParams.put("action", action);
        actionParams.put("params", params);
        return new TaskAction(PLUGIN_EXEC, actionParams);
    }

    private List<String> compileInputs(List<String> srcs, List<String> classpath) {
        List<String> inputs = new ArrayList<>(srcs);
        inputs.addAll(classpath);
        return ImmutableList.copyOf(inputs);
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
        List<String> srcs,
        List<String> classpath,
        String classesDir,
        String jar,
        boolean registerBuild
    ) {
        static final String DEFAULT_RELEASE = "17";
        static final List<String> DEFAULT_SRCS = ImmutableList.of("src/main/java/**/*.java");
        static final String DEFAULT_BUILD_DIR = "build";
        static final String DEFAULT_CLASSES_DIR = DEFAULT_BUILD_DIR + "/classes/java/main";
        static final String DEFAULT_JAR = DEFAULT_BUILD_DIR + "/libs/app.jar";

        static JavaConfig from(Map<String, Object> fields) {
            FieldMap values = new FieldMap(fields);
            String name = values.string("name", "app");
            String buildDir = values.string("build_dir", DEFAULT_BUILD_DIR);
            String classesDir = values.string("classes_dir", buildDir + "/classes/java/main");
            return new JavaConfig(
                name,
                values.string("release", DEFAULT_RELEASE),
                values.list("srcs", DEFAULT_SRCS),
                values.list("classpath", ImmutableList.of()),
                classesDir,
                values.string("jar", buildDir + "/libs/" + name + ".jar"),
                values.bool("register_build", true)
            );
        }
    }
}
