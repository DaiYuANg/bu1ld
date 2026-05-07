package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import com.google.common.collect.ImmutableMap;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

final class FieldMap {
    private final Map<String, Object> fields;

    FieldMap(Map<String, Object> fields) {
        this.fields = fields == null ? ImmutableMap.of() : fields;
    }

    String string(String name, String fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        Object value = fields.get(name);
        if (value instanceof String text) {
            return text;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be string");
    }

    List<String> list(String name, List<String> fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        Object value = fields.get(name);
        if (value instanceof String text) {
            return ImmutableList.of(text);
        }
        if (value instanceof List<?> items) {
            List<String> result = new ArrayList<>(items.size());
            for (Object item : items) {
                if (item instanceof String text) {
                    result.add(text);
                    continue;
                }
                throw new IllegalArgumentException("field \"" + name + "\" must be list");
            }
            return ImmutableList.copyOf(result);
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be list");
    }

    boolean bool(String name, boolean fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        Object value = fields.get(name);
        if (value instanceof Boolean enabled) {
            return enabled;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be bool");
    }
}
