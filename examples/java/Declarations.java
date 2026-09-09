/**
 * Declarations holds the bindings Java places at the top level it has: a class
 * body. A static field binds once per program, exactly as a package-level
 * constant does elsewhere; an instance field describes the type instead, and is
 * left to read_struct.
 */
public final class Declarations {
  /** MAX caps the retry loop. */
  public static final int MAX = 3;

  /** counter is mutable shared state, so it binds as a variable. */
  private static int counter = 0;

  /** width belongs to an instance and is not part of the file's surface. */
  private int width;

  /** sum adds two numbers, so the class also holds one callable. */
  public static int sum(int a, int b) {
    return a + b;
  }
}
