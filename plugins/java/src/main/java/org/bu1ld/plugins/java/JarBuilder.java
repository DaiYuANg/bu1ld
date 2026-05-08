package org.bu1ld.plugins.java;

import io.avaje.inject.Component;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.jar.Attributes;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import java.util.jar.Manifest;
import lombok.val;
import org.apache.commons.io.FileUtils;
import org.apache.commons.io.FilenameUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class JarBuilder {
    ExecuteResult create(ExecuteRequest request) throws IOException {
        val spec = JarSpec.from(request.params());
        val workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        val roots = jarRoots(workDir, spec);
        val output = workDir.resolve(spec.out()).normalize();
        if (roots.isEmpty() && spec.roots().isEmpty()) {
            throw new IllegalStateException("classes directory does not exist: " + spec.classes());
        }

        FileUtils.forceMkdirParent(output.toFile());
        val manifest = new Manifest();
        manifest.getMainAttributes().put(Attributes.Name.MANIFEST_VERSION, "1.0");
        int[] count = {0};
        try (val jar = new JarOutputStream(Files.newOutputStream(output), manifest)) {
            val entries = new LinkedHashSet<String>();
            for (val root : roots) {
                try (var stream = Files.walk(root)) {
                    for (val file : stream.filter(Files::isRegularFile).sorted().toList()) {
                        val entryName = FilenameUtils.separatorsToUnix(root.relativize(file).toString());
                        if (!entries.add(entryName)) {
                            continue;
                        }
                        val entry = new JarEntry(entryName);
                        entry.setTime(Files.getLastModifiedTime(file).toMillis());
                        jar.putNextEntry(entry);
                        Files.copy(file, jar);
                        jar.closeEntry();
                        count[0]++;
                    }
                }
            }
        }
        return new ExecuteResult("created jar " + spec.out() + " with " + count[0] + " file(s)\n");
    }

    private List<Path> jarRoots(Path workDir, JarSpec spec) {
        val roots = spec.roots().isEmpty() ? List.of(spec.classes()) : spec.roots();
        val result = new ArrayList<Path>(roots.size());
        for (val root : roots) {
            val path = workDir.resolve(root).normalize();
            if (Files.isDirectory(path)) {
                result.add(path);
            }
        }
        return result;
    }

    private record JarSpec(String classes, List<String> roots, String out) {
        static JarSpec from(java.util.Map<String, Object> params) {
            val fields = new FieldMap(params);
            return new JarSpec(
                fields.string("classes", JavaDefaults.CLASSES_DIR),
                fields.list("roots", List.of()),
                fields.string("out", JavaDefaults.jar("app"))
            );
        }
    }
}
