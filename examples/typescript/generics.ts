/**
 * Container holds a single value of type T.
 * @typeParam T the stored element type
 */
interface Container<T> {
  /** get returns the stored value. */
  get(): T;
  /** set replaces the stored value. */
  set(value: T): void;
}

/** Box is a mutable Container backed by a field. */
class Box<T> implements Container<T> {
  constructor(private value: T) {}

  get(): T {
    return this.value;
  }

  set(value: T): void {
    this.value = value;
  }
}

/** firstOf returns the first element of an array, or undefined. */
function firstOf<T>(items: T[]): T | undefined {
  return items[0];
}

// mapList applies fn to every element, returning a new array.
const mapList = <T, U>(items: T[], fn: (item: T) => U): U[] => items.map(fn);
