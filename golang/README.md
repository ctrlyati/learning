# 🐹 Go (Golang) Interview Prep Course

A structured, self-paced course to go from **zero to interview-ready in Go**. Each module covers one topic with clear explanations, code examples, and interview tips.

---

## 📚 Course Modules

| # | Topic | Focus Areas |
|---|-------|-------------|
| 01 | [Go Basics](./01_go_basics.md) | Setup, syntax, variables, types, constants, fmt |
| 02 | [Control Flow](./02_control_flow.md) | if/else, switch, for loops, range, goto |
| 03 | [Functions](./03_functions.md) | Params, returns, variadic, closures, defer, init |
| 04 | [Pointers](./04_pointers.md) | Pointers, dereferencing, nil, pass-by-value vs reference |
| 05 | [Data Structures](./05_data_structures.md) | Arrays, slices, maps, make, append, delete |
| 06 | [Structs & Methods](./06_structs_methods.md) | Struct definition, embedding, methods, tags |
| 07 | [Interfaces](./07_interfaces.md) | Interface definition, implicit impl, empty interface, type assertion |
| 08 | [Error Handling](./08_error_handling.md) | error type, custom errors, errors.Is/As, panic/recover |
| 09 | [Goroutines & Concurrency](./09_goroutines_concurrency.md) | Goroutines, WaitGroup, Mutex, race conditions |
| 10 | [Channels](./10_channels.md) | Buffered/unbuffered, select, done pattern, pipelines |
| 11 | [Packages & Modules](./11_packages_modules.md) | go mod, imports, visibility, init, build tags |
| 12 | [Testing](./12_testing.md) | Unit tests, table-driven tests, benchmarks, mocks |
| 13 | [Standard Library](./13_standard_library.md) | fmt, strings, strconv, os, io, net/http, encoding/json, time |
| 14 | [Patterns & Best Practices](./14_patterns_best_practices.md) | Functional options, context, generics, SOLID in Go |
| 15 | [Interview Q&A](./15_interview_qa.md) | 50+ common Go interview questions with answers |

---

## ⏱️ Suggested 4-Week Timeline

| Week | Modules | Hours/Day |
|------|---------|-----------|
| Week 1 | 01 → 05 — Language Fundamentals | 1–2 hrs |
| Week 2 | 06 → 09 — OOP & Concurrency | 1–2 hrs |
| Week 3 | 10 → 13 — Channels, Modules & Stdlib | 1–2 hrs |
| Week 4 | 14 → 15 + Practice | 2–3 hrs |

---

## 🛠️ Getting Started

**Install Go:**
```bash
brew install go           # macOS
# or download from https://go.dev/dl/
```

**Verify your installation:**
```bash
go version
```

**Create a new project to practice:**
```bash
mkdir go-practice && cd go-practice
go mod init github.com/yourname/go-practice
touch main.go
go run main.go
```

---

## 💡 Core Mental Models

- **Simple by design** — fewer features means more readable, maintainable code.
- **Composition over inheritance** — use embedding and interfaces instead of class hierarchies.
- **Concurrency is first-class** — goroutines and channels are built into the language.
- **Errors are values** — no exceptions; error handling is explicit and intentional.
- **The compiler is your ally** — unused variables and imports are compile errors, catching bugs early.
- **Fast compilation** — builds in seconds even for large projects.

---

## 📖 Recommended Resources

- [A Tour of Go](https://go.dev/tour/) — Official interactive introduction
- [Effective Go](https://go.dev/doc/effective_go) — Official best practices guide
- [Go by Example](https://gobyexample.com/) — Annotated code snippets for every concept
- [Go Playground](https://play.golang.org/) — Run Go code in the browser, no setup needed
- [awesome-go](https://github.com/avelino/awesome-go) — Curated list of Go libraries and tools

---

## 🗺️ Full Roadmap

See [00_roadmap.md](./00_roadmap.md) for the complete learning path and roadmap overview.

---

*Happy learning! 🚀*
