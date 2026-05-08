package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import java.util.List;

final class JavaDefaults {
    static final String RELEASE = "17";
    static final String BUILD_DIR = "build";
    static final List<String> SOURCES = ImmutableList.of("src/main/java/**/*.java");
    static final List<String> SOURCE_ROOTS = ImmutableList.of("src/main/java");
    static final List<String> RESOURCES = ImmutableList.of("src/main/resources/**");
    static final List<String> RESOURCE_ROOTS = ImmutableList.of("src/main/resources");
    static final List<String> REPOSITORIES = ImmutableList.of("https://repo.maven.apache.org/maven2");
    static final List<String> TEST_LAUNCHER_DEPENDENCIES = ImmutableList.of("org.junit.platform:junit-platform-launcher:1.11.4");
    static final String CLASSES_DIR = BUILD_DIR + "/classes/java/main";
    static final String RESOURCES_DIR = BUILD_DIR + "/resources/main";
    static final String JAVADOC_DIR = BUILD_DIR + "/docs/javadoc";
    static final String LOCAL_REPOSITORY = "~/.m2/repository";

    private JavaDefaults() {
    }

    static String jar(String name) {
        return BUILD_DIR + "/libs/" + name + ".jar";
    }

    static String sourcesJar(String name) {
        return BUILD_DIR + "/libs/" + name + "-sources.jar";
    }

    static String javadocJar(String name) {
        return BUILD_DIR + "/libs/" + name + "-javadoc.jar";
    }

    static List<String> sources(String sourceSet) {
        return ImmutableList.of("src/" + sourceSet + "/java/**/*.java");
    }

    static List<String> sourceRoots(String sourceSet) {
        return ImmutableList.of("src/" + sourceSet + "/java");
    }

    static List<String> resources(String sourceSet) {
        return ImmutableList.of("src/" + sourceSet + "/resources/**");
    }

    static List<String> resourceRoots(String sourceSet) {
        return ImmutableList.of("src/" + sourceSet + "/resources");
    }

    static String classesDir(String sourceSet) {
        return BUILD_DIR + "/classes/java/" + sourceSet;
    }

    static String resourcesDir(String sourceSet) {
        return BUILD_DIR + "/resources/" + sourceSet;
    }

    static String javadocDir(String sourceSet) {
        if ("main".equals(sourceSet)) {
            return JAVADOC_DIR;
        }
        return BUILD_DIR + "/docs/javadoc/" + sourceSet;
    }

    static String testResultsDir(String sourceSet) {
        return BUILD_DIR + "/test-results/" + sourceSet;
    }
}
