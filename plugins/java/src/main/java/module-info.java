module org.bu1ld.plugins.java {
    requires static lombok;

    requires org.apache.commons.lang3;
    requires org.apache.commons.io;
    requires com.google.common;
    requires io.avaje.inject;
    requires com.fasterxml.jackson.databind;
    requires org.slf4j;
    requires jul.to.slf4j;
    requires ch.qos.logback.classic;
    requires java.compiler;
    requires jdk.compiler;

    provides io.avaje.inject.spi.InjectExtension with org.bu1ld.plugins.java.JavaModule;

    exports org.bu1ld.plugins.java;
}
