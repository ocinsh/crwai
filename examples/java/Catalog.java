package com.example.demo;

import java.util.ArrayList;
import java.util.List;

/**
 * Catalog is a deliberately large file (>= 20 addressable symbols) used to
 * exercise list_signatures on a realistic source. Every member is deterministic:
 * no randomness, no clock, no network.
 */
public class Catalog {

    private final List<Item> items = new ArrayList<>();
    private String owner;

    /** Builds an empty catalog owned by nobody. */
    public Catalog() {
        this.owner = "";
    }

    /** Builds an empty catalog owned by the given owner. */
    public Catalog(String owner) {
        this.owner = owner;
    }

    public void add(Item item) {
        items.add(item);
    }

    public boolean remove(String sku) {
        return items.removeIf(i -> i.sku().equals(sku));
    }

    public int size() {
        return items.size();
    }

    public boolean isEmpty() {
        return items.isEmpty();
    }

    public Item find(String sku) {
        for (Item i : items) {
            if (i.sku().equals(sku)) {
                return i;
            }
        }
        return null;
    }

    public int totalCents() {
        int sum = 0;
        for (Item i : items) {
            sum += i.priceCents();
        }
        return sum;
    }

    public String owner() {
        return owner;
    }

    public void setOwner(String owner) {
        this.owner = owner;
    }

    public List<Item> itemsByCategory(Category category) {
        List<Item> out = new ArrayList<>();
        for (Item i : items) {
            if (i.category() == category) {
                out.add(i);
            }
        }
        return out;
    }
}

/**
 * Item is a single catalog entry.
 */
record Item(String sku, int priceCents, Category category) {

    public boolean isFree() {
        return priceCents == 0;
    }

    public String label() {
        return sku + " (" + category + ")";
    }
}

/**
 * Category groups items.
 */
enum Category {
    BOOKS,
    TOOLS,
    FOOD;

    public boolean perishable() {
        return this == FOOD;
    }
}

/**
 * Priced is anything that exposes a price in cents.
 */
interface Priced {

    int priceCents();

    default boolean isFree() {
        return priceCents() == 0;
    }
}
