/// Bodyless top-level declarations used to exercise the Dart listing.

/// kMax caps the retry loop.
const int kMax = 3;

/// greeting is final: it binds once at run time, which makes it a variable
/// rather than a compile-time constant.
final greeting = 'declarations';

/// counter is mutable.
var counter = 0;

/// Handler is a type alias.
typedef Handler = void Function(int);

/// sum adds two numbers, so the file also holds one callable.
int sum(int a, int b) => a + b;
