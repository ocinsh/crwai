"""Geometry helpers used to exercise the Python language reader/writer.

This module mixes documented and undocumented module-level functions, classes
with methods (to exercise SymbolID.Container), and names that are reused across
containers (to exercise disambiguation).
"""


def area(width, height):
    """Return the area of a rectangle."""
    return width * height


def describe(name):
    return "shape: " + name


def perimeter(width, height):
    """Return the rectangle perimeter.

    The perimeter is twice the sum of the two sides.
    """
    return 2 * (width + height)


class Rectangle:
    """A rectangle defined by its width and height."""

    def __init__(self, width, height):
        self.width = width
        self.height = height

    def area(self):
        """Return the rectangle area."""
        return self.width * self.height

    def describe(self):
        return "rectangle"


class Circle:
    """A circle defined by its radius."""

    def __init__(self, radius):
        self.radius = radius

    def area(self):
        return 3 * self.radius * self.radius
