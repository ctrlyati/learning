# 02 — Values, Types, Coercion, Equality

> **Goal:** Predict the result of every weird operator, comparison, and conversion JavaScript can throw at you — because those weirdnesses *are* the language.

---

## 1. The Type System — Mental Model

JavaScript has **8 types**. Memorize this list:

**Primitives (immutable, compared by value):**
1. `undefined`
2. `null`
3. `boolean`
4. `number` (IEEE-754 double)
5. `bigint` (arbitrary-precision integers, suffix `n`)
6. `string`
7. `symbol`

**Reference (mutable, compared by identity):**
8. `object` (includes arrays, functions, dates, maps, sets, regex, errors — *everything* not primitive)

```js
typeof undefined;        // "undefined"
typeof null;             // "object" ← legacy bug
typeof true;             // "boolean"
typeof 42;               // "number"
typeof 9007199254740993n;// "bigint"
typeof "hello";          // "string"
typeof Symbol("id");     // "symbol"
typeof {};               // "object"
typeof [];               // "object"  ← arrays are objects
typeof function(){};     // "function" ← but functions are still objects
```

The `typeof null === "object"` quirk is the most famous bug in JS. ECMA discussed fixing it; it would break the web. It stays.

### Variables hold values, not types
JS variables are *typeless boxes*; the **value** has a type.

```js
let x = 1;       // x holds a number
x = "one";       // now x holds a string — totally legal
x = { v: 1 };    // now an object reference
```

---

## 2. Coercion — Under the Hood

Coercion is JS automatically converting a value from one type to another. There are two kinds:

- **Explicit:** you call `Number(x)`, `String(x)`, `Boolean(x)`.
- **Implicit:** the language does it for you when operators mix types.

### The three coercion targets
Every value can be coerced to one of: `string`, `number`, or `boolean`.

#### To Boolean
The **falsy** values (memorize this list — it's short):
```
false, 0, -0, 0n, "", null, undefined, NaN
```
Everything else is truthy. Including `"0"`, `"false"`, `[]`, `{}`.

```js
Boolean("");      // false
Boolean("0");     // true   ← "0" the string is truthy
Boolean([]);      // true   ← empty array is truthy
Boolean({});      // true
Boolean(NaN);     // false
```

#### To Number
```js
Number("");       // 0    ← yes, really
Number("  42 "); // 42
Number("42abc"); // NaN
Number(true);    // 1
Number(false);   // 0
Number(null);    // 0
Number(undefined); // NaN
Number([]);      // 0    ← []  → "" → 0
Number([42]);    // 42   ← [42] → "42" → 42
Number([1,2]);   // NaN  ← [1,2] → "1,2" → NaN
Number({});      // NaN
```

#### To String
```js
String(42);          // "42"
String(null);        // "null"
String(undefined);   // "undefined"
String([1,2,3]);     // "1,2,3"   ← Array.prototype.join(",")
String({});          // "[object Object]"
String({a:1});       // "[object Object]"  (not JSON!)
```

### The `+` operator — string OR number
The **only** arithmetic operator that prefers strings. If either operand is a string (or coerces to one via an object), `+` concatenates. Otherwise it adds.

```js
1 + 2;        // 3
"1" + 2;      // "12"
1 + "2";      // "12"
1 + 2 + "3";  // "33"   ← left-to-right: (1+2)+"3"
"1" + 2 + 3;  // "123"
[] + [];      // ""     ← both → ""
[] + {};      // "[object Object]"
{} + [];      // 0      ← in some contexts; statement-position {} is a block!
```

The `{} + []` one is notorious. In a statement position, `{}` parses as an empty *block*, then `+[]` is a unary plus that coerces `[]` to `0`. In an expression context (`( {} + [] )`), it's `"[object Object]"`. Welcome.

All other arithmetic operators (`-`, `*`, `/`, `%`, `**`) coerce to number.
```js
"5" - 1;   // 4
"5" * "2"; // 10
```

---

## 3. Equality — `==` vs `===` (and `Object.is`)

Three kinds of equality. You should know them cold.

### `===` — strict equality
No coercion. Same type AND same value.

```js
1 === 1;      // true
"1" === 1;    // false  (different types)
NaN === NaN;  // false  ← NaN is never equal to anything, including itself
+0 === -0;    // true
```

### `==` — loose equality (the infamous one)
Coerces operands to the *same type* before comparing, with a specific algorithm:

- If types match → use `===`.
- `null == undefined` → **true** (and only each other).
- Number vs string → string is coerced to number.
- Boolean vs anything → boolean is coerced to number first.
- Object vs primitive → object is coerced to primitive (via `valueOf`/`toString`).

```js
null == undefined;   // true
null == 0;           // false  ← null only equals undefined and itself
"" == 0;             // true   ← "" → 0
"0" == false;        // true   ← false → 0, "0" → 0
[] == false;         // true   ← [] → "" → 0, false → 0
[1] == 1;            // true   ← [1] → "1" → 1
[1,2] == "1,2";      // true
NaN == NaN;          // false
```

**Practical rule:** use `===` always, EXCEPT one idiomatic shortcut:
```js
if (value == null) {
  // Catches both null and undefined. Common, accepted.
}
```

### `Object.is` — strictest of all
Like `===`, but with two fixes:
```js
Object.is(NaN, NaN);  // true
Object.is(+0, -0);    // false
```
Use it when you specifically need to distinguish `+0` / `-0`, or treat `NaN` as equal to itself (rare; React's reconciler uses it).

---

## 4. Practical Application — A Coercion-Safe Validator

Build a tiny `toNumberStrict` that refuses the silly cases:

```js
function toNumberStrict(input) {
  // Reject the historically bad coercions explicitly
  if (input === null) return NaN;
  if (typeof input === "boolean") return NaN;
  if (Array.isArray(input)) return NaN;
  if (typeof input === "object") return NaN;
  if (typeof input === "string") {
    const trimmed = input.trim();
    if (trimmed === "") return NaN;
    // Number() will accept "0x1f", "1e3", etc. Use parseFloat for stricter.
    const n = Number(trimmed);
    return Number.isFinite(n) ? n : NaN;
  }
  if (typeof input === "number") {
    return Number.isFinite(input) ? input : NaN;
  }
  if (typeof input === "bigint") return Number(input);
  return NaN;
}

// Tests
console.log(toNumberStrict("42"));      // 42
console.log(toNumberStrict(""));        // NaN  (vs Number("") → 0)
console.log(toNumberStrict([42]));      // NaN  (vs Number([42]) → 42)
console.log(toNumberStrict(true));      // NaN  (vs Number(true) → 1)
console.log(toNumberStrict(null));      // NaN  (vs Number(null) → 0)
console.log(toNumberStrict("  3.14 ")); // 3.14
console.log(toNumberStrict(Infinity)); // NaN
```

This kind of defensive parsing is what you do at every external input boundary (HTTP body, query string, env var) in real systems.

---

## 5. Common Mistakes & Gotchas

- **Floating point:** `0.1 + 0.2 === 0.3` is **false**. It's `0.30000000000000004`. Use a tolerance, or `Math.fround`, or libraries like `decimal.js` for money.
- **`parseInt` without a radix:** `parseInt("08")` was `0` in old JS (octal). Always pass `parseInt(x, 10)`. Or just use `Number(x)`.
- **`JSON.stringify` drops things silently:** `undefined` values, functions, and symbols are *omitted* (in objects) or become `null` (in arrays).
   ```js
   JSON.stringify({ a: undefined, b: () => {}, c: 1 }); // '{"c":1}'
   JSON.stringify([1, undefined, () => {}, 3]);         // '[1,null,null,3]'
   ```
- **`Number.MAX_SAFE_INTEGER` (2^53 - 1):** integers above this lose precision. Use `bigint` for IDs from databases.
- **`==` chain illusions:** `1 == "1" == true` is `(1 == "1") == true` → `true == true` → `true`. Looks meaningful, isn't.
- **`typeof` for arrays:** returns `"object"`. Use `Array.isArray(x)`.
- **`typeof null`:** `"object"`. Use `x === null` or `x == null` (catches undefined too).
- **String/number coercion in templates:** `` `${{a:1}}` `` gives `"[object Object]"`. Stringify objects explicitly: `` `${JSON.stringify(obj)}` ``.

```js
// The hall of fame "wat"
[] + [];          // ""
[] + {};          // "[object Object]"
true + true;      // 2
"5" - "2";        // 3
"5" + - "2";      // "5-2"
NaN === NaN;      // false
0.1 + 0.2;        // 0.30000000000000004
typeof typeof 1;  // "string"  ← typeof always returns a string
```

---

## 🎯 Key Takeaways

- **8 types**, only one of which (`object`) is by-reference. Internalize the list.
- **`===` by default**, `== null` as the only acceptable loose-equality idiom.
- **`+` is the lone coercion-to-string operator;** every other arithmetic op coerces to number.
- **Memorize the 7 falsy values.** Everything else is truthy — including `[]` and `{}`.
- **At input boundaries, validate explicitly.** Don't trust JS coercion to do the right thing on user input — write your own strict parsers.

---

*← [01 Setup & Runtimes](./01_setup_and_runtime.md) | [next → 03 Variables, Scope, Hoisting, TDZ](./03_variables_scope_hoisting.md)*
