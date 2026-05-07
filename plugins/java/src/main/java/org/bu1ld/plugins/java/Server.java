package org.bu1ld.plugins.java;

import io.avaje.inject.Component;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import org.eclipse.lsp4j.jsonrpc.Launcher;

import static org.bu1ld.plugins.java.Protocol.ConfigureParams;
import static org.bu1ld.plugins.java.Protocol.ConfigureResult;
import static org.bu1ld.plugins.java.Protocol.ExpandParams;
import static org.bu1ld.plugins.java.Protocol.ExpandResult;
import static org.bu1ld.plugins.java.Protocol.ExecuteParams;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;
import static org.bu1ld.plugins.java.Protocol.MetadataResult;

@Component
final class Server implements PluginJsonRpcService {
    private final Plugin plugin;

    Server(Plugin plugin) {
        this.plugin = plugin;
    }

    void serve(InputStream input, OutputStream output) throws IOException {
        Launcher<PluginHost> launcher = Launcher.createLauncher(this, PluginHost.class, input, output);
        try {
            launcher.startListening().get();
        } catch (InterruptedException err) {
            Thread.currentThread().interrupt();
            throw new IOException("interrupted while serving plugin JSON-RPC", err);
        } catch (ExecutionException err) {
            throw new IOException("serve plugin JSON-RPC", err.getCause());
        }
    }

    @Override
    public CompletableFuture<MetadataResult> metadata() {
        return CompletableFuture.completedFuture(new MetadataResult(plugin.metadata()));
    }

    @Override
    public CompletableFuture<ConfigureResult> configure(ConfigureParams params) {
        return CompletableFuture.completedFuture(new ConfigureResult(plugin.configure(params.config())));
    }

    @Override
    public CompletableFuture<ExpandResult> expand(ExpandParams params) {
        return CompletableFuture.completedFuture(new ExpandResult(plugin.expand(params.invocation())));
    }

    @Override
    public CompletableFuture<ExecuteResult> execute(ExecuteParams params) {
        try {
            return CompletableFuture.completedFuture(plugin.execute(params.request()));
        } catch (Exception err) {
            return CompletableFuture.failedFuture(err);
        }
    }

    private interface PluginHost {
    }
}
