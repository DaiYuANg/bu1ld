package org.bu1ld.plugins.java;

import io.avaje.inject.Component;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import lombok.val;
import org.apache.commons.io.FileUtils;

import static org.bu1ld.plugins.java.Protocol.ExecuteRequest;
import static org.bu1ld.plugins.java.Protocol.ExecuteResult;

@Component
final class ResourceProcessor {
    ExecuteResult process(ExecuteRequest request) throws IOException {
        val spec = ResourceSpec.from(request.params());
        val workDir = Path.of(request.workDir()).toAbsolutePath().normalize();
        val outputDir = workDir.resolve(spec.out()).normalize();
        val resources = ProjectFiles.expand(workDir, spec.resources());

        FileUtils.forceMkdir(outputDir.toFile());
        int count = 0;
        for (val resource : resources) {
            val target = outputDir.resolve(relativeResourcePath(workDir, resource, spec.resourceRoots())).normalize();
            FileUtils.forceMkdirParent(target.toFile());
            FileUtils.copyFile(resource.toFile(), target.toFile());
            count++;
        }

        if (count == 0) {
            return new ExecuteResult("no Java resources matched\n");
        }
        return new ExecuteResult("processed " + count + " Java resource file(s) to " + spec.out() + "\n");
    }

    private Path relativeResourcePath(Path workDir, Path resource, List<String> resourceRoots) {
        for (val root : resourceRoots) {
            val resourceRoot = workDir.resolve(root).normalize();
            if (Files.isDirectory(resourceRoot) && resource.startsWith(resourceRoot)) {
                return resourceRoot.relativize(resource);
            }
        }
        return workDir.relativize(resource);
    }

    private record ResourceSpec(List<String> resources, List<String> resourceRoots, String out) {
        static ResourceSpec from(java.util.Map<String, Object> params) {
            val fields = new FieldMap(params);
            return new ResourceSpec(
                fields.list("resources", JavaDefaults.RESOURCES),
                fields.list("resource_roots", JavaDefaults.RESOURCE_ROOTS),
                fields.string("out", JavaDefaults.RESOURCES_DIR)
            );
        }
    }
}
