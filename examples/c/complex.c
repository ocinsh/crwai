// complex.c is a realistic, deterministic C file with many top-level symbols,
// used to exercise list_signatures on a non-trivial file (>= 20 symbols).

// Vec2 is a 2D vector of doubles.
struct Vec2 {
    double x;
    double y;
};

// Stats accumulates a running count and sum.
typedef struct {
    long count;
    double sum;
} Stats;

// imax returns the larger of two ints.
int imax(int a, int b) {
    return a > b ? a : b;
}

// imin returns the smaller of two ints.
int imin(int a, int b) {
    return a < b ? a : b;
}

// iabs returns the absolute value of n.
int iabs(int n) {
    return n < 0 ? -n : n;
}

// iclamp constrains v to the inclusive range [lo, hi].
int iclamp(int v, int lo, int hi) {
    return imax(lo, imin(v, hi));
}

// isum returns the sum of the first n elements of xs.
int isum(const int *xs, int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        total += xs[i];
    }
    return total;
}

// igcd returns the greatest common divisor of a and b.
int igcd(int a, int b) {
    while (b != 0) {
        int t = b;
        b = a % b;
        a = t;
    }
    return a;
}

// ilcm returns the least common multiple of a and b.
int ilcm(int a, int b) {
    if (a == 0 || b == 0) {
        return 0;
    }
    return iabs(a / igcd(a, b) * b);
}

// ipow returns base raised to a non-negative integer exponent.
long ipow(int base, int exp) {
    long result = 1;
    for (int i = 0; i < exp; i++) {
        result *= base;
    }
    return result;
}

// ifac returns n! for small non-negative n.
long ifac(int n) {
    long result = 1;
    for (int i = 2; i <= n; i++) {
        result *= i;
    }
    return result;
}

// is_even reports whether n is even.
int is_even(int n) {
    return n % 2 == 0;
}

// is_odd reports whether n is odd.
int is_odd(int n) {
    return !is_even(n);
}

// dmax returns the larger of two doubles.
double dmax(double a, double b) {
    return a > b ? a : b;
}

// dmin returns the smaller of two doubles.
double dmin(double a, double b) {
    return a < b ? a : b;
}

// vec2_add returns the component-wise sum of a and b.
struct Vec2 vec2_add(struct Vec2 a, struct Vec2 b) {
    struct Vec2 r;
    r.x = a.x + b.x;
    r.y = a.y + b.y;
    return r;
}

// vec2_scale returns v scaled by factor f.
struct Vec2 vec2_scale(struct Vec2 v, double f) {
    struct Vec2 r;
    r.x = v.x * f;
    r.y = v.y * f;
    return r;
}

// vec2_dot returns the dot product of a and b.
double vec2_dot(struct Vec2 a, struct Vec2 b) {
    return a.x * b.x + a.y * b.y;
}

// stats_init zeroes a Stats accumulator.
void stats_init(Stats *s) {
    s->count = 0;
    s->sum = 0.0;
}

// stats_push records one observation x.
void stats_push(Stats *s, double x) {
    s->count += 1;
    s->sum += x;
}

// stats_mean returns the mean, or 0.0 when no observations were recorded.
double stats_mean(const Stats *s) {
    if (s->count == 0) {
        return 0.0;
    }
    return s->sum / (double) s->count;
}

// fib returns the n-th Fibonacci number iteratively.
long fib(int n) {
    long a = 0, b = 1;
    for (int i = 0; i < n; i++) {
        long t = a + b;
        a = b;
        b = t;
    }
    return a;
}

// sign returns -1, 0, or 1 according to the sign of n.
int sign(int n) {
    if (n > 0) {
        return 1;
    }
    if (n < 0) {
        return -1;
    }
    return 0;
}
