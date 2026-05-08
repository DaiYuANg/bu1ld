package example.junit;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

final class GreetingTest {
    @Test
    void formatsGreeting() {
        assertEquals("Hello, bu1ld!", Greeting.message("bu1ld"));
    }
}
