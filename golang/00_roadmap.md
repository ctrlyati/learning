# 🗺️ Golang Learning Roadmap — Interview Prep Edition

> **Goal:** Go from zero to interview-ready in Go. Each numbered file covers one topic with explanations, code examples, and interview tips.

---

## 📋 Learning Path

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

## ⏱️ Suggested Timeline

| Week | Topics | Hours/Day |
|------|--------|-----------|
| Week 1 | 01 → 05 | 1–2 hrs |
| Week 2 | 06 → 09 | 1–2 hrs |
| Week 3 | 10 → 13 | 1–2 hrs |
| Week 4 | 14 → 15 + Practice | 2–3 hrs |

---

## 🛠️ Setup

```bash
# Install Go
brew install go           # macOS
# or download from https://go.dev/dl/

# Verify
go version

# Create a new project
mkdir myproject && cd myproject
go mod init github.com/yourname/myproject

# Run
go run main.go

# Build
go build -o myapp .
```

---

## 💡 Key Mental Models

1. **Go is simple by design** — fewer features, more readability.
2. **Composition over inheritance** — use embedding + interfaces instead of class hierarchies.
3. **Concurrency is a first-class citizen** — goroutines and channels are built-in.
4. **Errors are values** — no exceptions, error handling is explicit.
5. **The compiler is strict** — unused variables and imports are compile errors.
6. **Fast compilation** — builds in seconds even for large projects.

---

## 📚 Recommended Resources

- [Go Tour](https://go.dev/tour/) — Official interactive intro
- [Effective Go](https://go.dev/doc/effective_go) — Official best practices
- [Go by Example](https://gobyexample.com/) — Code snippets for every concept
- [Go Playground](https://play.golang.org/) — Run code in the browser
- [awesome-go](https://github.com/avelino/awesome-go) — Curated Go libraries

---

*Good luck with your interviews, Yati! 🚀*
