module org.bu1ld.plugins.java {
    requires static lombok;

    requires org.apache.commons.lang3;
    requires org.apache.commons.io;
    requires com.google.common;
    requires com.google.gson;
    requires io.avaje.inject;
    requires org.eclipse.lsp4j.jsonrpc;
    requires org.apache.maven.resolver.supplier;
    requires java.compiler;
    requires java.logging;
    requires jdk.compiler;
    requires jdk.javadoc;

    provides io.avaje.inject.spi.InjectExtension with org.bu1ld.plugins.java.JavaModule;

    exports org.bu1ld.plugins.java;
    opens org.bu1ld.plugins.java;
}
