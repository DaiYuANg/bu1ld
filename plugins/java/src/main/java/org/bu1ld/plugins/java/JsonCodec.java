package org.bu1ld.plugins.java;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.avaje.inject.Component;
import java.io.IOException;

import static org.bu1ld.plugins.java.Protocol.ExpandParams;
import static org.bu1ld.plugins.java.Protocol.ConfigureParams;
import static org.bu1ld.plugins.java.Protocol.ExecuteParams;
import static org.bu1ld.plugins.java.Protocol.Request;
import static org.bu1ld.plugins.java.Protocol.Response;

@Component
final class JsonCodec {
    private final ObjectMapper mapper;

    JsonCodec(ObjectMapper mapper) {
        this.mapper = mapper;
    }

    Request readRequest(String line) throws IOException {
        return mapper.readValue(line, Request.class);
    }

    String writeResponse(Response response) throws IOException {
        return mapper.writeValueAsString(response);
    }

    ExpandParams readExpandParams(Object params) {
        return mapper.convertValue(params, ExpandParams.class);
    }

    ConfigureParams readConfigureParams(Object params) {
        return mapper.convertValue(params, ConfigureParams.class);
    }

    ExecuteParams readExecuteParams(Object params) {
        return mapper.convertValue(params, ExecuteParams.class);
    }
}
