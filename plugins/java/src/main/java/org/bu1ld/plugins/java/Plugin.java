package org.bu1ld.plugins.java;

import java.util.List;

import static org.bu1ld.plugins.java.Protocol.Metadata;
import static org.bu1ld.plugins.java.Protocol.PluginConfig;
import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;
import static org.bu1ld.plugins.java.Protocol.TaskSpec;

interface Plugin {
    Metadata metadata();

    List<TaskSpec> configure(PluginConfig config);

    List<TaskSpec> expand(Invocation invocation);

    ExecuteResult execute(ExecuteRequest request) throws Exception;
}
