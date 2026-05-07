package org.bu1ld.plugins.java;

import io.avaje.inject.BeanScope;
import java.io.IOException;
import java.util.logging.Handler;
import java.util.logging.Level;
import java.util.logging.LogManager;
import java.util.logging.Logger;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public final class Bu1ldJavaPlugin {
    public static void main(String[] args) throws IOException {
        configureLogging();
        try (BeanScope scope = BeanScope.builder().build()) {
            scope.get(Server.class).serve(System.in, System.out);
        }
    }

    private static void configureLogging() {
        Logger root = LogManager.getLogManager().getLogger("");
        if (root == null) {
            return;
        }
        root.setLevel(Level.WARNING);
        for (Handler handler : root.getHandlers()) {
            handler.setLevel(Level.WARNING);
        }
    }
}
