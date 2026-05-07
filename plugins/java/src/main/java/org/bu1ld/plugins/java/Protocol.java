package org.bu1ld.plugins.java;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;
import java.util.Map;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public final class Protocol {
    @NoArgsConstructor(access = AccessLevel.PRIVATE)
    public static final class Method {
        public static final String METADATA = "metadata";
        public static final String CONFIGURE = "configure";
        public static final String EXPAND = "expand";
        public static final String EXECUTE = "execute";
    }

    public static record Request(long id, String method, Object params) {
    }

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public static record Response(long id, Object result, ResponseError error) {
        public static Response result(long id, Object result) {
            return new Response(id, result, null);
        }

        public static Response error(long id, String message) {
            return new Response(id, null, new ResponseError(message));
        }
    }

    public static record ResponseError(String message) {
    }

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public static record Metadata(
        String id,
        String namespace,
        List<RuleSchema> rules,
        List<FieldSchema> configFields,
        boolean autoConfigure
    ) {
    }

    public static record MetadataResult(Metadata metadata) {
    }

    public static record FieldSchema(String name, String type, boolean required) {
    }

    public static record RuleSchema(String name, List<FieldSchema> fields) {
    }

    public static record ExpandParams(Invocation invocation) {
    }

    public static record ExpandResult(List<TaskSpec> tasks) {
    }

    public static record PluginConfig(String namespace, Map<String, Object> fields) {
    }

    public static record ConfigureParams(PluginConfig config) {
    }

    public static record ConfigureResult(List<TaskSpec> tasks) {
    }

    public static record ExecuteParams(ExecuteRequest request) {
    }

    public static record ExecuteRequest(String namespace, String action, String workDir, Map<String, Object> params) {
    }

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public static record ExecuteResult(String output) {
    }

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public static record TaskSpec(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
        List<String> command,
        TaskAction action
    ) {
    }

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public static record TaskAction(String kind, Map<String, Object> params) {
    }
}
