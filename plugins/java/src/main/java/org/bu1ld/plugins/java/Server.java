package org.bu1ld.plugins.java;

import io.avaje.inject.Component;
import java.io.BufferedReader;
import java.io.BufferedWriter;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.io.OutputStreamWriter;
import java.nio.charset.StandardCharsets;
import lombok.extern.slf4j.Slf4j;
import org.apache.commons.lang3.StringUtils;

import static org.bu1ld.plugins.java.Protocol.Method.CONFIGURE;
import static org.bu1ld.plugins.java.Protocol.Method.METADATA;
import static org.bu1ld.plugins.java.Protocol.Method.EXECUTE;
import static org.bu1ld.plugins.java.Protocol.Method.EXPAND;
import static org.bu1ld.plugins.java.Protocol.ConfigureParams;
import static org.bu1ld.plugins.java.Protocol.ConfigureResult;
import static org.bu1ld.plugins.java.Protocol.ExpandParams;
import static org.bu1ld.plugins.java.Protocol.ExpandResult;
import static org.bu1ld.plugins.java.Protocol.ExecuteParams;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;
import static org.bu1ld.plugins.java.Protocol.MetadataResult;
import static org.bu1ld.plugins.java.Protocol.Request;
import static org.bu1ld.plugins.java.Protocol.Response;

@Component
@Slf4j
final class Server {
    private final Plugin plugin;
    private final JsonCodec json;

    Server(Plugin plugin, JsonCodec json) {
        this.plugin = plugin;
        this.json = json;
    }

    void serve(InputStream input, OutputStream output) throws IOException {
        BufferedReader reader = new BufferedReader(new InputStreamReader(input, StandardCharsets.UTF_8));
        BufferedWriter writer = new BufferedWriter(new OutputStreamWriter(output, StandardCharsets.UTF_8));
        String line;
        while ((line = reader.readLine()) != null) {
            if (StringUtils.isBlank(line)) {
                continue;
            }
            writer.write(json.writeResponse(handle(line)));
            writer.newLine();
            writer.flush();
        }
    }

    private Response handle(String line) {
        long id = 0;
        try {
            Request request = json.readRequest(line);
            id = request.id();
            log.debug("handling plugin RPC request id={} method={}", id, request.method());
            return switch (request.method()) {
                case METADATA -> Response.result(id, new MetadataResult(plugin.metadata()));
                case CONFIGURE -> Response.result(id, configure(request.params()));
                case EXPAND -> Response.result(id, expand(request.params()));
                case EXECUTE -> Response.result(id, execute(request.params()));
                default -> Response.error(id, "unknown plugin method \"" + request.method() + "\"");
            };
        } catch (RuntimeException | IOException err) {
            log.warn("plugin RPC request failed id={}", id, err);
            return Response.error(id, err.getMessage());
        }
    }

    private ExpandResult expand(Object params) {
        ExpandParams parsed = json.readExpandParams(params);
        return new ExpandResult(plugin.expand(parsed.invocation()));
    }

    private ConfigureResult configure(Object params) {
        ConfigureParams parsed = json.readConfigureParams(params);
        return new ConfigureResult(plugin.configure(parsed.config()));
    }

    private ExecuteResult execute(Object params) {
        try {
            ExecuteParams parsed = json.readExecuteParams(params);
            return plugin.execute(parsed.request());
        } catch (Exception err) {
            throw new IllegalStateException(err.getMessage(), err);
        }
    }
}
