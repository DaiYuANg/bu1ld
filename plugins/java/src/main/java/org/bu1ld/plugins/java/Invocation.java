package org.bu1ld.plugins.java;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

record Invocation(String namespace, String rule, String target, Map<String, Object> fields) {
    Invocation {
        if (fields == null) {
            fields = Collections.emptyMap();
        }
    }

    boolean hasField(String name) {
        return fields.containsKey(name);
    }

    String requiredString(String name) {
        if (!fields.containsKey(name)) {
            throw new IllegalArgumentException(namespace + "." + rule + " requires field \"" + name + "\"");
        }
        return string(fields.get(name), namespace + "." + rule + "." + name);
    }

    String optionalString(String name, String fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        return string(fields.get(name), namespace + "." + rule + "." + name);
    }

    List<String> optionalList(String name, List<String> fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        Object value = fields.get(name);
        if (value instanceof String text) {
            return List.of(text);
        }
        if (value instanceof List<?> items) {
            List<String> result = new ArrayList<>(items.size());
            for (Object item : items) {
                result.add(string(item, namespace + "." + rule + "." + name));
            }
            return result;
        }
        throw new IllegalArgumentException(namespace + "." + rule + " field \"" + name + "\" must be list");
    }

    boolean optionalBool(String name, boolean fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        Object value = fields.get(name);
        if (value instanceof Boolean enabled) {
            return enabled;
        }
        throw new IllegalArgumentException(namespace + "." + rule + " field \"" + name + "\" must be bool");
    }

    private static String string(Object value, String name) {
        if (value instanceof String text) {
            return text;
        }
        throw new IllegalArgumentException(name + " must be string");
    }
}
