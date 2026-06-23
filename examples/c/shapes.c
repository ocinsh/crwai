// Point is a 2D integer point.
struct Point {
    int x;
    int y;
};

/* Rect is an axis-aligned rectangle defined by two corners. */
struct Rect {
    struct Point min;
    struct Point max;
};

// Size is a width/height pair defined via typedef.
typedef struct {
    int w;
    int h;
} Size;

// area returns the area of the rectangle r.
int area(struct Rect r) {
    int w = r.max.x - r.min.x;
    int h = r.max.y - r.min.y;
    return w * h;
}

// café: a Unicode comment to exercise non-ASCII bytes in doc extraction.
struct Pixel {
    int r;
    int g;
    int b;
};
