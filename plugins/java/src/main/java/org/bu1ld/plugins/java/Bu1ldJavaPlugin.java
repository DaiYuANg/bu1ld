package org.bu1ld.plugins.java;

import io.avaje.inject.BeanScope;
import java.io.IOException;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.slf4j.bridge.SLF4JBridgeHandler;

@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public final class Bu1ldJavaPlugin {
    public static void main(String[] args) throws IOException {
        SLF4JBridgeHandler.removeHandlersForRootLogger();
        SLF4JBridgeHandler.install();
        log.info("starting bu1ld Java plugin RPC server");
        try (BeanScope scope = BeanScope.builder().build()) {
            scope.get(Server.class).serve(System.in, System.out);
        }
        log.info("stopped bu1ld Java plugin RPC server");
    }
}
