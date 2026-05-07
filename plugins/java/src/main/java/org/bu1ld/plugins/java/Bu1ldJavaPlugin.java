package org.bu1ld.plugins.java;

import io.avaje.inject.BeanScope;
import java.io.IOException;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

@NoArgsConstructor(access = AccessLevel.PRIVATE)
public final class Bu1ldJavaPlugin {
    public static void main(String[] args) throws IOException {
        try (BeanScope scope = BeanScope.builder().build()) {
            scope.get(Server.class).serve(System.in, System.out);
        }
    }
}
