// list is a singly linked list node of ints.
struct list {
    int value;
    struct list *next;
};

// list constructs an empty list (a NULL head). It shares the name "list" with
// the struct above; the two are disambiguated by SymbolKind, since C has no
// container for free functions.
struct list *list(void) {
    return 0;
}
