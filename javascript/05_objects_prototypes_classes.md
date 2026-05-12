# 05 — Objects, Prototypes, Classes

> **Goal:** See through the `class` keyword to the prototype chain underneath, and use objects idiomatically (property descriptors, getters/setters, `Object.freeze`, etc.).

---

## 1. Objects — Mental Model

An object is an unordered set of **properties**. A property is a key (string or symbol) → value (anything).

```js
const user = { name: "Ada", age: 36, "favorite color": "teal" };
console.log(user.name);              // dot access (key must be valid identifier)
console.log(user["favorite color"]); // bracket access (any string)

// Computed keys
const key = "age";
console.log(user[key]); // 36

// Shorthand
const name = "Ada", age = 36;
const u2 = { name, age };  // { name: "Ada", age: 36 }

// Method shorthand
const obj = { greet() { return "hi"; } };

// Symbol keys (won't show up in for..in or JSON.stringify)
const id = Symbol("id");
user[id] = 42;
```

Property keys are **always** strings or symbols. Numbers get coerced: `obj[1]` is the same as `obj["1"]`.

---

## 2. The Prototype Chain — Under the Hood

Every object has a hidden internal slot `[[Prototype]]` (exposed via `Object.getPrototypeOf` or the legacy `__proto__`) pointing to another object — its **prototype**. When you read a property, JS searches the chain:

```
obj  →  obj.[[Prototype]]  →  ...  →  Object.prototype  →  null
```

```js
const animal = { eats: true };
const dog = Object.create(animal);
dog.barks = true;

console.log(dog.barks); // true  (own property)
console.log(dog.eats);  // true  (inherited from animal)
console.log(Object.getPrototypeOf(dog) === animal); // true
console.log(Object.hasOwn(dog, "eats")); // false — own check
```

Writes always create an **own** property; they never mutate the prototype:
```js
dog.eats = false;
console.log(dog.eats);    // false (own)
console.log(animal.eats); // true  (untouched)
```

### Functions and `prototype`
Every (non-arrow) function has a `.prototype` property — an object that becomes the prototype of instances created with `new`.

```js
function Dog(name) { this.name = name; }
Dog.prototype.bark = function () { return `${this.name} says woof`; };

const rex = new Dog("Rex");
rex.bark();                                  // "Rex says woof"
Object.getPrototypeOf(rex) === Dog.prototype; // true
rex instanceof Dog;                           // true (walks the chain)
```

### `class` is sugar over this
```js
class Dog {
  constructor(name) { this.name = name; }
  bark() { return `${this.name} says woof`; }
}
```
is essentially identical to the prototype version above. `class` adds:
- `extends` for clean inheritance
- `super(...)` for parent calls
- `static` members on the constructor
- private fields with `#`
- Enforces `new` (calling `Dog()` throws)

```js
class Animal {
  constructor(name) { this.name = name; }
  describe() { return `I am ${this.name}`; }
  static kingdom() { return "Animalia"; }
}

class Dog extends Animal {
  #secret = "buried bones in yard";
  constructor(name, breed) {
    super(name);          // must call before using `this`
    this.breed = breed;
  }
  describe() {            // override + super
    return super.describe() + ` the ${this.breed}`;
  }
  reveal() { return this.#secret; }
}

const d = new Dog("Rex", "lab");
console.log(d.describe());     // "I am Rex the lab"
console.log(Dog.kingdom());    // "Animalia"
console.log(d.reveal());       // "buried..."
// d.#secret;                  // SyntaxError — outside the class body
```

---

## 3. Property Descriptors, Getters/Setters, Freezing

Each property has a **descriptor** with metadata:

```js
const obj = { x: 1 };
Object.getOwnPropertyDescriptor(obj, "x");
// { value: 1, writable: true, enumerable: true, configurable: true }
```

Define them explicitly:
```js
Object.defineProperty(obj, "y", {
  value: 2,
  writable: false,    // can't reassign
  enumerable: false,  // hidden from for..in / Object.keys
  configurable: false,// can't redefine or delete
});
```

### Accessor properties (getters/setters)
```js
const user = {
  firstName: "Ada",
  lastName: "Lovelace",
  get fullName() { return `${this.firstName} ${this.lastName}`; },
  set fullName(s) {
    [this.firstName, this.lastName] = s.split(" ");
  },
};
console.log(user.fullName); // "Ada Lovelace"
user.fullName = "Grace Hopper";
console.log(user.firstName); // "Grace"
```

In classes:
```js
class Temp {
  #c;
  constructor(c) { this.#c = c; }
  get celsius() { return this.#c; }
  get fahrenheit() { return this.#c * 9/5 + 32; }
  set fahrenheit(f) { this.#c = (f - 32) * 5/9; }
}
```

### Object.freeze / seal / preventExtensions
```js
const cfg = Object.freeze({ port: 3000 });
cfg.port = 4000;            // silently fails (or throws in strict mode)
cfg.newKey = "x";           // also fails
// Note: shallow! cfg.nested = { a: 1 }; cfg.nested.a = 2 still works.
```

For deep freeze, walk the tree yourself or use a library.

### Mixing — useful in practice
```js
function deepFreeze(obj) {
  for (const k of Object.keys(obj)) {
    if (obj[k] && typeof obj[k] === "object") deepFreeze(obj[k]);
  }
  return Object.freeze(obj);
}
const config = deepFreeze({ db: { host: "localhost", port: 5432 } });
```

### Iterating object properties
```js
const obj = { a: 1, b: 2, c: 3 };
Object.keys(obj);    // ["a","b","c"]
Object.values(obj);  // [1,2,3]
Object.entries(obj); // [["a",1],["b",2],["c",3]]
for (const [k, v] of Object.entries(obj)) console.log(k, v);

// for...in walks the chain (avoid unless you mean it)
for (const k in obj) console.log(k);
```

### `Object.assign` and spread
```js
const merged = Object.assign({}, base, override);
const merged2 = { ...base, ...override }; // preferred — ES2018+
```
Both are **shallow**. Nested objects are shared.

---

## 4. Practical Application — A `User` Class with Validation, Equality, JSON

```js
class User {
  #email;

  constructor({ id, name, email, createdAt = new Date() }) {
    if (!id) throw new TypeError("User: id required");
    if (typeof name !== "string" || !name) throw new TypeError("User: name required");
    this.id = id;
    this.name = name;
    this.email = email; // goes through the setter
    this.createdAt = createdAt;
    Object.freeze(this); // make instances shallowly immutable
  }

  set email(value) {
    if (!/^.+@.+\..+$/.test(value)) throw new TypeError(`Invalid email: ${value}`);
    this.#email = value.toLowerCase();
  }
  get email() { return this.#email; }

  equals(other) {
    return other instanceof User && other.id === this.id;
  }

  // Customize JSON.stringify behavior
  toJSON() {
    return {
      id: this.id,
      name: this.name,
      email: this.#email,
      createdAt: this.createdAt.toISOString(),
    };
  }

  static fromJSON(json) {
    return new User({ ...json, createdAt: new Date(json.createdAt) });
  }
}

const u = new User({ id: 1, name: "Ada", email: "Ada@Example.COM" });
console.log(u.email);             // "ada@example.com"
console.log(JSON.stringify(u));   // {"id":1,"name":"Ada","email":"ada@example.com","createdAt":"..."}
const u2 = User.fromJSON(JSON.parse(JSON.stringify(u)));
console.log(u.equals(u2));        // true
```

Patterns shown: validation in setters, private fields, frozen instances, `toJSON`/`fromJSON`, structural equality via id, static factory methods.

---

## 5. Common Mistakes & Gotchas

- **`for...in` walks the prototype chain.** Use `Object.keys`/`entries` for own properties only.
- **Numeric/symbol keys ordering:** integer-like keys are sorted ascending in iteration, then string keys in insertion order, then symbol keys. So `{2:"a",1:"b","x":"c",0:"d"}` iterates `0,1,2,"x"`.
- **`hasOwnProperty` is itself on the prototype** and can be shadowed. Use `Object.hasOwn(obj, "key")` (ES2022).
- **Spreading an object loses non-enumerable and symbol-keyed properties.** Spread is for plain data.
- **`JSON.stringify` skips `undefined`, functions, symbols.** `toJSON` lets you intervene.
- **Calling `super()` after using `this` in a subclass constructor** throws ReferenceError. `super()` must come first.
- **Accidentally mutating a prototype:**
  ```js
  Array.prototype.first = function () { return this[0]; }; // DON'T
  ```
  Pollutes everywhere. This is also part of how **prototype pollution** vulnerabilities work (module 17).
- **Comparing objects with `===`:** compares references, not contents. `{a:1} === {a:1}` is `false`. Use a deep-equal lib if you need value equality.
- **`Object.keys` skips symbol keys.** Use `Object.getOwnPropertySymbols` or `Reflect.ownKeys`.

```js
// "Wat"
const obj = {};
obj.__proto__ === Object.prototype; // true
Object.create(null).__proto__;      // undefined  ← null-prototype object
({} instanceof Object);             // true
(Object.create(null)) instanceof Object; // false
```
Null-prototype objects (`Object.create(null)`) are useful as safe maps — they can't be poisoned via `__proto__`.

---

## 🎯 Key Takeaways

- **Inheritance in JS is the prototype chain.** `class` is sugar; the engine is still walking `[[Prototype]]` links.
- **Reads walk the chain; writes always create own properties.** This single rule explains most prototype "surprises."
- **Property descriptors give precise control** over writability, enumerability, configurability — useful for frameworks and security-sensitive code.
- **Use `Object.hasOwn` and `Object.entries`,** not `hasOwnProperty` or `for...in`, in modern code.
- **`class` brings real features beyond syntax** — private fields (`#`), `super`, static, and `new`-enforcement. Use them; avoid manual prototype manipulation in new code.

---

*← [04 Functions, Closures, `this`](./04_functions_closures_this.md) | [next → 06 Arrays & Iteration](./06_arrays_iteration.md)*
