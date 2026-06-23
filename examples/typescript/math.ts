// add returns the sum of two numbers.
function add(a: number, b: number): number {
  return a + b;
}

function subtract(a: number, b: number): number {
  return a - b;
}

/**
 * multiply returns the product of two numbers.
 * @param a the first factor
 * @param b the second factor
 * @returns the product a * b
 */
function multiply(a: number, b: number): number {
  return a * b;
}

// square returns n squared, as an arrow function bound to a const.
const square = (n: number): number => n * n;

export const identity = <T>(value: T): T => value;
