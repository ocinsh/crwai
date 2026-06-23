// add returns the sum of two numbers.
function add(a, b) {
  return a + b;
}

function subtract(a, b) {
  return a - b;
}

/**
 * multiply returns the product of two numbers.
 * It does not mutate its arguments.
 */
function multiply(a, b) {
  return a * b;
}

// square returns n squared via an arrow function.
const square = (n) => n * n;

module.exports = { add, subtract, multiply, square };
