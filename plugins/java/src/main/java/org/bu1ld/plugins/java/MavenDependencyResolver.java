package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import io.avaje.inject.Component;
import java.lang.reflect.Method;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import lombok.val;

@Component
final class MavenDependencyResolver {
    List<Path> resolve(
        Path workDir,
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        boolean offline
    ) throws Exception {
        val artifacts = resolveArtifacts(workDir, repositories, dependencies, localRepository, offline);
        val paths = new ArrayList<Path>(artifacts.size());
        for (val artifact : artifacts) {
            paths.add(artifact.path());
        }
        return ImmutableList.copyOf(paths);
    }

    List<ResolvedArtifact> resolveArtifacts(
        Path workDir,
        List<String> repositories,
        List<String> dependencies,
        String localRepository,
        boolean offline
    ) throws Exception {
        if (dependencies.isEmpty()) {
            return ImmutableList.of();
        }

        val runtime = ResolverRuntime.create(resolveLocalRepository(workDir, localRepository), offline);
        try (runtime) {
            val artifacts = new LinkedHashMap<String, ResolvedArtifact>();
            for (val dependency : dependencies) {
                for (val artifact : runtime.resolve(dependency, remoteRepositories(repositories))) {
                    artifacts.putIfAbsent(artifact.coordinate(), artifact);
                }
            }
            return ImmutableList.copyOf(artifacts.values());
        }
    }

    private Path resolveLocalRepository(Path workDir, String localRepository) {
        val repository = localRepository == null || localRepository.isBlank()
            ? JavaDefaults.LOCAL_REPOSITORY
            : localRepository;
        if (repository.startsWith("~/") || repository.startsWith("~\\")) {
            return Path.of(System.getProperty("user.home"), repository.substring(2)).normalize();
        }
        val path = Path.of(repository);
        if (path.isAbsolute()) {
            return path.normalize();
        }
        return workDir.resolve(path).normalize();
    }

    private List<String> remoteRepositories(List<String> repositories) {
        return ImmutableList.copyOf(repositories.isEmpty() ? JavaDefaults.REPOSITORIES : repositories);
    }

    private static final class ResolverRuntime implements AutoCloseable {
        private final Object system;
        private final Object session;

        private ResolverRuntime(Object system, Object session) {
            this.system = system;
            this.session = session;
        }

        static ResolverRuntime create(Path localRepository, boolean offline) throws Exception {
            val supplier = type("org.eclipse.aether.supplier.RepositorySystemSupplier")
                .getConstructor()
                .newInstance();
            val system = method(supplier.getClass(), "get").invoke(supplier);
            val sessionBuilderSupplier = type("org.eclipse.aether.supplier.SessionBuilderSupplier")
                .getConstructor(type("org.eclipse.aether.RepositorySystem"))
                .newInstance(system);
            val sessionBuilder = method(sessionBuilderSupplier.getClass(), "get").invoke(sessionBuilderSupplier);
            method(sessionBuilder.getClass(), "withLocalRepositoryBaseDirectories", Path[].class)
                .invoke(sessionBuilder, (Object) new Path[]{localRepository});
            method(sessionBuilder.getClass(), "setOffline", boolean.class).invoke(sessionBuilder, offline);
            val session = method(sessionBuilder.getClass(), "build").invoke(sessionBuilder);
            return new ResolverRuntime(system, session);
        }

        List<ResolvedArtifact> resolve(String dependency, List<String> repositories) throws Exception {
            val request = dependencyRequest(dependency, repositories);
            val result = method(system.getClass(), "resolveDependencies",
                type("org.eclipse.aether.RepositorySystemSession"),
                type("org.eclipse.aether.resolution.DependencyRequest")
            ).invoke(system, session, request);

            val artifactResults = (List<?>) method(result.getClass(), "getArtifactResults").invoke(result);
            val artifacts = new ArrayList<ResolvedArtifact>(artifactResults.size());
            val seenPaths = new LinkedHashSet<Path>();
            for (val artifactResult : artifactResults) {
                val artifact = method(artifactResult.getClass(), "getArtifact").invoke(artifactResult);
                val path = (Path) method(artifact.getClass(), "getPath").invoke(artifact);
                if (path != null && seenPaths.add(path)) {
                    artifacts.add(new ResolvedArtifact(artifactCoordinate(artifact), path.normalize()));
                }
            }
            return artifacts;
        }

        private Object dependencyRequest(String dependency, List<String> repositories) throws Exception {
            val collect = type("org.eclipse.aether.collection.CollectRequest").getConstructor().newInstance();
            val artifact = type("org.eclipse.aether.artifact.DefaultArtifact")
                .getConstructor(String.class)
                .newInstance(dependency);
            val root = type("org.eclipse.aether.graph.Dependency")
                .getConstructor(type("org.eclipse.aether.artifact.Artifact"), String.class)
                .newInstance(artifact, "compile");
            method(collect.getClass(), "setRoot", type("org.eclipse.aether.graph.Dependency")).invoke(collect, root);
            for (int index = 0; index < repositories.size(); index++) {
                method(collect.getClass(), "addRepository", type("org.eclipse.aether.repository.RemoteRepository"))
                    .invoke(collect, remoteRepository("repo-" + index, repositories.get(index)));
            }
            return type("org.eclipse.aether.resolution.DependencyRequest")
                .getConstructor(
                    type("org.eclipse.aether.collection.CollectRequest"),
                    type("org.eclipse.aether.graph.DependencyFilter")
                )
                .newInstance(collect, classpathFilter());
        }

        private Object remoteRepository(String id, String url) throws Exception {
            val builder = type("org.eclipse.aether.repository.RemoteRepository$Builder")
                .getConstructor(String.class, String.class, String.class)
                .newInstance(id, "default", url);
            return method(builder.getClass(), "build").invoke(builder);
        }

        private Object classpathFilter() throws Exception {
            return type("org.eclipse.aether.util.filter.DependencyFilterUtils")
                .getMethod("classpathFilter", String[].class)
                .invoke(null, (Object) new String[]{"compile", "runtime"});
        }

        @Override
        public void close() throws Exception {
            try {
                method(session.getClass(), "close").invoke(session);
            } finally {
                method(system.getClass(), "shutdown").invoke(system);
            }
        }

        private static Class<?> type(String name) throws ClassNotFoundException {
            return Class.forName(name);
        }

        private static Method method(Class<?> type, String name, Class<?>... parameterTypes) throws NoSuchMethodException {
            return type.getMethod(name, parameterTypes);
        }

        private static String artifactCoordinate(Object artifact) throws Exception {
            val groupId = (String) method(artifact.getClass(), "getGroupId").invoke(artifact);
            val artifactId = (String) method(artifact.getClass(), "getArtifactId").invoke(artifact);
            val extension = (String) method(artifact.getClass(), "getExtension").invoke(artifact);
            val classifier = (String) method(artifact.getClass(), "getClassifier").invoke(artifact);
            val version = (String) method(artifact.getClass(), "getVersion").invoke(artifact);
            if (classifier == null || classifier.isBlank()) {
                return groupId + ":" + artifactId + ":" + extension + ":" + version;
            }
            return groupId + ":" + artifactId + ":" + extension + ":" + classifier + ":" + version;
        }
    }
}

record ResolvedArtifact(String coordinate, Path path) {
}
