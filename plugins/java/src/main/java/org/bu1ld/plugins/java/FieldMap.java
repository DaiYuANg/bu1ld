package org.bu1ld.plugins.java;

import com.google.common.collect.ImmutableList;
import com.google.common.collect.ImmutableMap;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import lombok.val;

final class FieldMap {
    private final Map<String, Object> fields;

    FieldMap(Map<String, Object> fields) {
        this.fields = fields == null ? ImmutableMap.of() : fields;
    }

    boolean has(String name) {
        return fields.containsKey(name);
    }

    String string(String name, String fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        val value = fields.get(name);
        if (value instanceof String text) {
            return text;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be string");
    }

    List<String> list(String name, List<String> fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        val value = fields.get(name);
        if (value instanceof String text) {
            return ImmutableList.of(text);
        }
        if (value instanceof List<?> items) {
            val result = new ArrayList<String>(items.size());
            for (val item : items) {
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
        val value = fields.get(name);
        if (value instanceof Boolean enabled) {
            return enabled;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be bool");
    }

    Map<String, Object> object(String name, Map<String, Object> fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        val value = fields.get(name);
        if (value instanceof Map<?, ?> items) {
            val result = new LinkedHashMap<String, Object>(items.size());
            for (val entry : items.entrySet()) {
                if (entry.getKey() instanceof String key) {
                    result.put(key, entry.getValue());
                    continue;
                }
                throw new IllegalArgumentException("field \"" + name + "\" must be object");
            }
            return result;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be object");
    }

    Map<String, String> stringMap(String name, Map<String, String> fallback) {
        if (!fields.containsKey(name)) {
            return fallback;
        }
        val value = fields.get(name);
        if (value instanceof Map<?, ?> items) {
            val result = new LinkedHashMap<String, String>(items.size());
            for (val entry : items.entrySet()) {
                if (entry.getKey() instanceof String key && entry.getValue() instanceof String item) {
                    result.put(key, item);
                    continue;
                }
                throw new IllegalArgumentException("field \"" + name + "\" must be object with string values");
            }
            return result;
        }
        throw new IllegalArgumentException("field \"" + name + "\" must be object");
    }
}
