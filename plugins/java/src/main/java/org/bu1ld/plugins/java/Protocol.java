package org.bu1ld.plugins.java;

import com.google.gson.annotations.SerializedName;
import java.util.List;
import java.util.Map;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public final class Protocol {
    public static record Metadata(
        String id,
        String namespace,
        List<RuleSchema> rules,
        @SerializedName("config_fields")
        List<FieldSchema> configFields,
        @SerializedName("auto_configure")
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

    public static record ExecuteRequest(String namespace, String action, @SerializedName("work_dir") String workDir, Map<String, Object> params) {
    }

    public static record ExecuteResult(String output) {
    }

    public static record TaskSpec(
        String name,
        List<String> deps,
        List<String> inputs,
        List<String> outputs,
        List<String> command,
        TaskAction action
    ) {
    }

    public static record TaskAction(String kind, Map<String, Object> params) {
    }
}
