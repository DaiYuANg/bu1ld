package org.bu1ld.plugins.java;

import io.avaje.inject.Component;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.jar.Attributes;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import java.util.jar.Manifest;
import lombok.extern.slf4j.Slf4j;
import org.apache.commons.io.FileUtils;
import org.apache.commons.io.FilenameUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
@Slf4j
final class JarBuilder {
    ExecuteResult create(ExecuteRequest request) throws IOException {
        JarSpec spec = JarSpec.from(request.params());
        Path workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        Path classesDir = workDir.resolve(spec.classes()).normalize();
        Path output = workDir.resolve(spec.out()).normalize();
        log.info("creating jar classes={} output={}", classesDir, output);
        if (!Files.isDirectory(classesDir)) {
            throw new IllegalStateException("classes directory does not exist: " + spec.classes());
        }

        FileUtils.forceMkdirParent(output.toFile());
        Manifest manifest = new Manifest();
        manifest.getMainAttributes().put(Attributes.Name.MANIFEST_VERSION, "1.0");
        int[] count = {0};
        try (JarOutputStream jar = new JarOutputStream(Files.newOutputStream(output), manifest);
            var stream = Files.walk(classesDir)) {
            for (Path file : stream.filter(Files::isRegularFile).sorted().toList()) {
                String entryName = FilenameUtils.separatorsToUnix(classesDir.relativize(file).toString());
                JarEntry entry = new JarEntry(entryName);
                entry.setTime(Files.getLastModifiedTime(file).toMillis());
                jar.putNextEntry(entry);
                Files.copy(file, jar);
                jar.closeEntry();
                count[0]++;
            }
        }
        log.info("created jar {} with {} file(s)", spec.out(), count[0]);
        return new ExecuteResult("created jar " + spec.out() + " with " + count[0] + " file(s)\n");
    }

    private record JarSpec(String classes, String out) {
        static JarSpec from(java.util.Map<String, Object> params) {
            FieldMap fields = new FieldMap(params);
            return new JarSpec(
                fields.string("classes", "build/classes/java/main"),
                fields.string("out", "build/libs/app.jar")
            );
        }
    }
}
