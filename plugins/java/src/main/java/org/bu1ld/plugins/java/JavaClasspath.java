package org.bu1ld.plugins.java;

import com.google.common.base.Joiner;
import java.io.File;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import lombok.val;

final class JavaClasspath {
    private JavaClasspath() {
    }

    static List<Path> resolve(Path workDir, List<String> classpath) {
        val entries = new ArrayList<Path>(classpath.size());
        for (val item : classpath) {
            entries.add(workDir.resolve(item).normalize());
        }
        return entries;
    }

    static String join(List<Path> entries) {
        return Joiner.on(File.pathSeparator).join(entries.stream().map(Path::toString).toList());
    }
}
