package example.junit;

public final class Greeting {
    private Greeting() {
    }

    public static String message(String name) {
        return "Hello, " + name + "!";
    }
}
