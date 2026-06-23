// A deliberately symbol-dense file: more than twenty top-level declarations to
// exercise list_signatures on a realistic file. All deterministic, no I/O.

/// Adds two integers.
int add(int a, int b) => a + b;

/// Subtracts b from a.
int subtract(int a, int b) => a - b;

int multiply(int a, int b) => a * b;

/// Integer division, truncating toward zero.
int divide(int a, int b) => a ~/ b;

int modulo(int a, int b) => a % b;

/// Raises base to a non-negative integer exponent.
int power(int base, int exp) {
  var result = 1;
  for (var i = 0; i < exp; i++) {
    result *= base;
  }
  return result;
}

int negate(int a) => -a;

int abs(int a) => a < 0 ? -a : a;

/// Returns the larger of two integers.
int max(int a, int b) => a > b ? a : b;

/// Returns the smaller of two integers.
int min(int a, int b) => a < b ? a : b;

int clamp(int v, int lo, int hi) => v < lo ? lo : (v > hi ? hi : v);

bool isEven(int n) => n % 2 == 0;

bool isOdd(int n) => n % 2 != 0;

/// Returns the factorial of a non-negative integer.
int factorial(int n) {
  var acc = 1;
  for (var i = 2; i <= n; i++) {
    acc *= i;
  }
  return acc;
}

int gcd(int a, int b) => b == 0 ? a : gcd(b, a % b);

int lcm(int a, int b) => (a ~/ gcd(a, b)) * b;

double mean(List<int> xs) => xs.isEmpty ? 0 : sum(xs) / xs.length;

/// Sums a list of integers.
int sum(List<int> xs) {
  var total = 0;
  for (final x in xs) {
    total += x;
  }
  return total;
}

int product(List<int> xs) {
  var total = 1;
  for (final x in xs) {
    total *= x;
  }
  return total;
}

/// Wraps a value into a single-element list (exercises generics).
List<T> singleton<T>(T value) => [value];

bool isPrime(int n) {
  if (n < 2) return false;
  for (var i = 2; i * i <= n; i++) {
    if (n % i == 0) return false;
  }
  return true;
}

int square(int n) => n * n;

int cube(int n) => n * n * n;
