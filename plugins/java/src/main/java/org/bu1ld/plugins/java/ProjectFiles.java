package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.regex.Pattern;
import org.apache.commons.io.FilenameUtils;

final class ProjectFiles {
    private ProjectFiles() {
    }

    static List<Path> expand(Path root, List<String> patterns) throws IOException {
        List<Path> files = new ArrayList<>();
        for (String pattern : patterns) {
            files.addAll(expandOne(root, pattern));
        }
        files.sort(Comparator.comparing(Path::toString));
        return ImmutableList.copyOf(files.stream().distinct().toList());
    }

    private static List<Path> expandOne(Path root, String pattern) throws IOException {
        if (!hasGlob(pattern)) {
            Path path = root.resolve(pattern).normalize();
            if (Files.isDirectory(path)) {
                try (var stream = Files.walk(path)) {
                    return stream.filter(Files::isRegularFile).sorted().toList();
                }
            }
            if (Files.isRegularFile(path)) {
                return ImmutableList.of(path);
            }
            return ImmutableList.of();
        }

        Path base = globBase(root, pattern);
        if (!Files.exists(base)) {
            return ImmutableList.of();
        }
        Pattern matcher = Pattern.compile(globRegex(pattern));
        try (var stream = Files.walk(base)) {
            return stream
                .filter(Files::isRegularFile)
                .filter(path -> matcher.matcher(toSlash(root.relativize(path))).matches())
                .sorted()
                .toList();
        }
    }

    private static Path globBase(Path root, String pattern) {
        int firstGlob = firstGlobIndex(pattern);
        if (firstGlob < 0) {
            return root.resolve(pattern).normalize();
        }
        int slash = Math.max(pattern.lastIndexOf('/', firstGlob), pattern.lastIndexOf('\\', firstGlob));
        if (slash < 0) {
            return root;
        }
        return root.resolve(pattern.substring(0, slash)).normalize();
    }

    private static boolean hasGlob(String pattern) {
        return firstGlobIndex(pattern) >= 0;
    }

    private static int firstGlobIndex(String pattern) {
        int result = -1;
        for (char marker : new char[]{'*', '?', '['}) {
            int index = pattern.indexOf(marker);
            if (index >= 0 && (result < 0 || index < result)) {
                result = index;
            }
        }
        return result;
    }

    private static String globRegex(String pattern) {
        String text = FilenameUtils.separatorsToUnix(pattern);
        StringBuilder regex = new StringBuilder("^");
        for (int i = 0; i < text.length(); i++) {
            char ch = text.charAt(i);
            if (ch == '*') {
                if (i + 1 < text.length() && text.charAt(i + 1) == '*') {
                    if (i + 2 < text.length() && text.charAt(i + 2) == '/') {
                        regex.append("(?:.*/)?");
                        i += 2;
                    } else {
                        regex.append(".*");
                        i++;
                    }
                } else {
                    regex.append("[^/]*");
                }
                continue;
            }
            if (ch == '?') {
                regex.append("[^/]");
                continue;
            }
            if ("\\.[]{}()+-^$|".indexOf(ch) >= 0) {
                regex.append('\\');
            }
            regex.append(ch);
        }
        return regex.append('$').toString();
    }

    private static String toSlash(Path path) {
        return FilenameUtils.separatorsToUnix(path.toString());
    }
}
