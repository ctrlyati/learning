# 06 — Arrays & Iteration

> **Goal:** Use arrays and the iteration protocol fluently — pick the right method, understand what mutates vs returns new, and write clean, expressive transformations.

---

## 1. Arrays — Mental Model

An array in JS is a special object with:
- A `length` property that auto-updates
- Numeric (string) keys
- `Array.prototype` methods (map, filter, reduce, etc.)

```js
const a = [1, 2, 3];
typeof a;            // "object"
Array.isArray(a);    // true
a.length;            // 3
a[10] = "sparse";    // legal — creates a sparse array
a.length;            // 11
console.log(a);      // [1,2,3, <7 empty items>, "sparse"]
```

Sparse arrays are weird — `forEach`/`map` skip empty slots, `for` loops don't. **Avoid creating sparse arrays.** Use `.fill()` or explicit `undefined`.

### Creating arrays
```js
[];                          // literal — preferred
new Array(3);                // [<3 empty items>] — sparse, almost never what you want
new Array(1, 2, 3);          // [1,2,3]
Array.of(3);                 // [3] — single arg safe
Array.from("abc");           // ["a","b","c"]
Array.from({ length: 3 }, (_, i) => i * 2); // [0,2,4]
[..."abc"];                  // ["a","b","c"]
```

`Array.from({length: n}, mapFn)` is the canonical "make a range":
```js
const range = (n) => Array.from({ length: n }, (_, i) => i);
range(5); // [0,1,2,3,4]
```

---

## 2. Mutating vs Non-Mutating — Under the Hood

**The single most important table** in this module. Memorize which methods change the array vs return a new one.

| Mutating | Non-mutating |
|----------|--------------|
| `push`, `pop`, `shift`, `unshift` | `concat`, `slice` |
| `splice` | `map`, `filter`, `flatMap` |
| `sort`, `reverse` | `toSorted`, `toReversed` (ES2023!) |
| `fill`, `copyWithin` | `with(i, v)` (ES2023) |

ES2023 added immutable counterparts so you don't have to spread-then-mutate:
```js
const arr = [3, 1, 2];
const sorted = arr.toSorted();   // [1,2,3]  — arr unchanged
const reversed = arr.toReversed();
const updated = arr.with(0, 99); // [99,1,2] — arr unchanged
```

Old idiom you'll still see:
```js
const sorted = [...arr].sort(); // copy first, then sort
```

### Iteration methods you must know cold

```js
const nums = [1, 2, 3, 4, 5];

nums.forEach((n) => console.log(n));           // side effects, no return
nums.map((n) => n * 2);                         // [2,4,6,8,10]
nums.filter((n) => n % 2 === 0);                // [2,4]
nums.reduce((acc, n) => acc + n, 0);            // 15
nums.reduceRight((acc, n) => acc + n, 0);       // same, right-to-left

nums.find((n) => n > 3);                        // 4 (or undefined)
nums.findIndex((n) => n > 3);                   // 3
nums.findLast((n) => n > 3);                    // 5  (ES2023)
nums.includes(3);                               // true
nums.indexOf(3);                                // 2 (or -1)
nums.some((n) => n > 4);                        // true
nums.every((n) => n > 0);                       // true

nums.flat();                                    // shallow flatten
[[1],[2,[3]]].flat();                           // [1, 2, [3]]
[[1],[2,[3]]].flat(Infinity);                   // [1,2,3]
nums.flatMap((n) => [n, n * 2]);                // [1,2,2,4,3,6,4,8,5,10]

nums.entries();                                 // iterator of [i, v]
nums.keys();                                    // iterator of indices
nums.values();                                  // iterator of values
```

### `reduce` — the swiss army knife
```js
// Sum
[1,2,3].reduce((a, b) => a + b, 0);           // 6
// Group by
const users = [{role:"a"},{role:"b"},{role:"a"}];
users.reduce((acc, u) => {
  (acc[u.role] ??= []).push(u);
  return acc;
}, {}); // { a: [{role:"a"},{role:"a"}], b: [{role:"b"}] }
// Or use the new built-in (ES2024):
Object.groupBy(users, (u) => u.role);
```

---

## 3. The Iteration Protocol

`for...of` works on **iterables** — anything with a `[Symbol.iterator]()` method that returns an **iterator** (an object with `next()` returning `{ value, done }`).

Built-in iterables: `Array`, `String`, `Map`, `Set`, `arguments`, `NodeList`, generators.

```js
for (const ch of "abc") console.log(ch); // a b c
for (const [k, v] of new Map([["x",1],["y",2]])) console.log(k, v);
```

### Custom iterables
```js
const range = {
  from: 1, to: 5,
  [Symbol.iterator]() {
    let i = this.from;
    const last = this.to;
    return {
      next() {
        return i <= last ? { value: i++, done: false } : { value: undefined, done: true };
      },
    };
  },
};
for (const n of range) console.log(n); // 1 2 3 4 5
console.log([...range]);               // [1,2,3,4,5] — spread works on any iterable
```

### Generators — iterators made easy
```js
function* fibs() {
  let [a, b] = [0, 1];
  while (true) {
    yield a;
    [a, b] = [b, a + b];
  }
}

const it = fibs();
console.log(it.next().value); // 0
console.log(it.next().value); // 1
console.log(it.next().value); // 1
// Or take with a helper:
function take(iter, n) {
  const out = [];
  for (const v of iter) { if (out.length === n) break; out.push(v); }
  return out;
}
console.log(take(fibs(), 8)); // [0,1,1,2,3,5,8,13]
```

ES2025 brings `Iterator.prototype` helpers (`map`, `filter`, `take`, etc.) so you can chain generators lazily:
```js
// In Node 22+, modern browsers
fibs().take(8).filter(n => n % 2 === 0).toArray(); // [0,2,8]
```

### `Map` vs object, `Set` vs array
- Use **`Map`** when keys aren't strings, when you'll add/remove a lot, or when iteration order by insertion matters.
- Use **`Set`** for unique values and O(1) `has()`.

```js
const seen = new Set();
const dedup = (arr) => arr.filter((x) => !seen.has(x) && seen.add(x));
// Or simpler:
const unique = [...new Set([1,2,2,3,3,3])]; // [1,2,3]
```

---

## 4. Practical Application — A Realistic Pipeline

Imagine processing a list of orders to compute revenue per customer, sorted, top 3.

```js
const orders = [
  { id: "o1", customerId: "c1", amount: 50, status: "paid" },
  { id: "o2", customerId: "c2", amount: 120, status: "pending" },
  { id: "o3", customerId: "c1", amount: 30, status: "paid" },
  { id: "o4", customerId: "c3", amount: 200, status: "paid" },
  { id: "o5", customerId: "c2", amount: 80, status: "paid" },
  { id: "o6", customerId: "c1", amount: 10, status: "refunded" },
  { id: "o7", customerId: "c3", amount: 70, status: "paid" },
];

const top3 = Object.entries(
  orders
    .filter((o) => o.status === "paid")
    .reduce((acc, o) => {
      acc[o.customerId] = (acc[o.customerId] ?? 0) + o.amount;
      return acc;
    }, {})
)
  .map(([customerId, total]) => ({ customerId, total }))
  .toSorted((a, b) => b.total - a.total)
  .slice(0, 3);

console.log(top3);
// [
//   { customerId: 'c3', total: 270 },
//   { customerId: 'c2', total: 80 },
//   { customerId: 'c1', total: 80 },
// ]
```

Notice:
- Pure pipeline; no mutation of `orders`.
- `toSorted` (ES2023) avoids the spread-then-sort copy.
- `??` for safe default in `reduce`.
- One line per logical step — readable.

For very large datasets, prefer a single `for...of` loop — `reduce`/`map`/`filter` create intermediate arrays. Below ~100k items it doesn't matter; above, profile.

---

## 5. Common Mistakes & Gotchas

- **`sort()` sorts as strings by default!**
  ```js
  [10, 1, 2].sort(); // [1, 10, 2] — string compare
  [10, 1, 2].sort((a, b) => a - b); // [1, 2, 10]
  ```
- **`map` ignoring `this`:**
  ```js
  arr.map(function (x) { return this.factor * x; }, { factor: 10 }); // 2nd arg sets `this`
  ```
  But arrow functions ignore the `thisArg`. Use closures:
  ```js
  const factor = 10;
  arr.map((x) => factor * x);
  ```
- **`forEach` cannot be broken out of.** No `break`, no `continue` (use `return` to skip current). Use `for...of` if you need control flow.
- **`async` callbacks in `forEach` don't wait:**
  ```js
  // BUG: doesn't await
  arr.forEach(async (x) => { await save(x); });
  // Use:
  for (const x of arr) await save(x);
  // Or in parallel:
  await Promise.all(arr.map(save));
  ```
- **Sparse-array surprises:** `[,,,].length === 3`. Methods like `map` skip holes; spread converts holes to `undefined`.
- **Mutating during iteration:**
  ```js
  for (let i = 0; i < arr.length; i++) {
    if (cond) arr.splice(i, 1); // skips next item — `length` shrunk
  }
  ```
  Iterate in reverse, or filter into a new array.
- **`reduce` without initial value** uses `arr[0]` as the accumulator and starts at index 1. Empty array + no initial → TypeError.
- **`Array.from(new Set(...))`** is a fine dedupe; `[...new Set(...)]` is the same and shorter.

```js
// "Wat"
[1, 2, 3].sort();              // [1, 2, 3]  ← coincidentally OK
[10, 2, 1].sort();             // [1, 10, 2] ← string sort bites
typeof [];                     // "object"
[] + [];                       // ""
[] == false;                   // true
[1] == 1;                      // true
[null].toString();             // ""   (null/undefined → "")
[undefined].toString();        // ""
[NaN].includes(NaN);           // true ← unlike indexOf which uses ===
```

---

## 🎯 Key Takeaways

- **Know the mutating vs non-mutating split.** Use ES2023 `toSorted`/`toReversed`/`with` to avoid copy-then-mutate.
- **Pick the right method:** `map` for transform, `filter` for select, `reduce` for fold, `find/findIndex` for search, `some/every` for tests.
- **`forEach` is for side effects.** It doesn't return anything and can't be broken out of. Don't `await` inside it.
- **Iterables and generators** are the protocol behind `for...of`, spread, and destructuring. Master them; they show up in async iteration too.
- **Use `Map`/`Set` when their semantics fit.** Object-as-map and array-as-set lead to bugs (prototype pollution, slow `includes`).

---

*← [05 Objects, Prototypes, Classes](./05_objects_prototypes_classes.md) | [next → 07 Control Flow, Destructuring, Spread](./07_control_flow_destructuring.md)*
