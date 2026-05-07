package org.bu1ld.plugins.java;

import java.util.concurrent.CompletableFuture;
import org.eclipse.lsp4j.jsonrpc.services.JsonRequest;

import static org.bu1ld.plugins.java.Protocol.ConfigureParams;
import static org.bu1ld.plugins.java.Protocol.ConfigureResult;
import static org.bu1ld.plugins.java.Protocol.ExecuteParams;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;
import static org.bu1ld.plugins.java.Protocol.ExpandParams;
import static org.bu1ld.plugins.java.Protocol.ExpandResult;
import static org.bu1ld.plugins.java.Protocol.MetadataResult;

interface PluginJsonRpcService {
    @JsonRequest
    CompletableFuture<MetadataResult> metadata();

    @JsonRequest
    CompletableFuture<ConfigureResult> configure(ConfigureParams params);

    @JsonRequest
    CompletableFuture<ExpandResult> expand(ExpandParams params);

    @JsonRequest
    CompletableFuture<ExecuteResult> execute(ExecuteParams params);
}
