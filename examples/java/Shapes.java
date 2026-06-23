package com.example.demo;

/**
 * Shape is something with a measurable area.
 */
public interface Shape {

    /** area returns the shape's area in square units. */
    double area();

    String name();
}

/**
 * Point is an immutable 2D coordinate.
 */
record Point(int x, int y) {
}

/**
 * Circle is a Shape defined by its radius.
 */
class Circle implements Shape {

    private final double radius;

    Circle(double radius) {
        this.radius = radius;
    }

    @Override
    public double area() {
        return Math.PI * radius * radius;
    }

    @Override
    public String name() {
        return "circle";
    }
}

/**
 * Town greets visitors. Its greet method is a deliberate homonym of
 * Greeter.greet, with a different container and a different signature, to
 * exercise SymbolID disambiguation.
 */
class Town {

    public String greet() {
        return "welcome to town";
    }
}
