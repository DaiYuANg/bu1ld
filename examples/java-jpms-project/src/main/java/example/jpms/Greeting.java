package example.jpms;

import org.apache.commons.lang3.StringUtils;

public final class Greeting {
    private Greeting() {
    }

    public static String message(String name) {
        return "Hello, " + StringUtils.capitalize(name) + "!";
    }
}
