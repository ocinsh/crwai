// makeCounter returns a function that increments a private counter on each call.
const makeCounter = function (start) {
  let n = start;
  return function () {
    n += 1;
    return n;
  };
};

// identity returns its single argument unchanged (block-bodied arrow).
const identity = (x) => {
  return x;
};

// pair builds a two-element array (expression-bodied arrow).
const pair = (a, b) => [a, b];

module.exports = { makeCounter, identity, pair };
