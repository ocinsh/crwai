package com.example.demo;

/**
 * Greeter builds friendly greetings.
 *
 * It exercises multi-line Javadoc on a class declaration so the
 * doc-extraction tests have a deterministic, exactly-known string.
 */
public class Greeter {

    private final String name;

    /** Builds a greeter bound to a name. */
    public Greeter(String name) {
        this.name = name;
    }

    // greet returns a salutation; this line comment is the doc.
    public String greet(String who) {
        return name + " greets " + who;
    }

    /**
     * identity returns its argument unchanged.
     * It is generic to exercise type parameters.
     */
    public <T> T identity(T value) {
        return value;
    }

    public String farewell(String who, int times) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < times; i++) {
            sb.append("bye ").append(who).append(' ');
        }
        return sb.toString().trim();
    }
}
