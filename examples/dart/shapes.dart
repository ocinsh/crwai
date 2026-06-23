/// Returns a generic, module-level description.
/// Shares its name with Shape.describe and Rectangle.describe so the
/// SymbolID.Container is what disambiguates them.
String describe() {
  return "a shape";
}

/// The base shape contract.
///
/// Maps to ReadInterface because it is an abstract class.
abstract class Shape {
  /// Computes the area of the shape.
  double area();

  /// Describes the shape in words.
  String describe();
}

/// A rectangle with a width and a height.
///
/// Maps to ReadStruct because it is a concrete class.
class Rectangle extends Shape {
  final double width;
  final double height;

  Rectangle(this.width, this.height);

  /// Computes width times height.
  @override
  double area() {
    return width * height;
  }

  /// Describes this rectangle.
  @override
  String describe() {
    return "rectangle";
  }

  /// Returns its single argument unchanged (exercises generics).
  T identity<T>(T value) {
    return value;
  }
}

/// Doubles every element using a nested local function (exercises closures).
List<int> doubleAll(List<int> xs) {
  int twice(int n) {
    return n * 2;
  }

  return xs.map(twice).toList();
}
