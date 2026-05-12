# 13 — Standard Library Highlights

> **Goal:** Know the most important standard library packages — interviewers expect you to navigate these confidently.

---

## 1. fmt — Formatting & I/O

```go
// Print
fmt.Print("no newline")
fmt.Println("with newline")
fmt.Printf("%s is %d years old\n", "Yati", 28)

// Format to string
s := fmt.Sprintf("pi = %.2f", 3.14159)

// Scan from stdin
var name string
fmt.Scan(&name)
fmt.Scanln(&name)
fmt.Scanf("%s", &name)

// Verbs
// %v  default, %+v struct with names, %#v Go syntax
// %d  decimal, %b binary, %x hex, %o octal
// %f  float, %e scientific, %g compact
// %s  string, %q quoted, %p pointer
// %t  bool, %T type
```

---

## 2. strings — String Manipulation

```go
import "strings"

s := "Hello, World!"

strings.Contains(s, "World")          // true
strings.HasPrefix(s, "Hello")          // true
strings.HasSuffix(s, "!")              // true
strings.Count(s, "l")                  // 3
strings.Index(s, "World")              // 7

strings.ToUpper(s)                     // "HELLO, WORLD!"
strings.ToLower(s)                     // "hello, world!"
strings.Title("hello world")           // "Hello World"

strings.TrimSpace("  hello  ")         // "hello"
strings.Trim("--hello--", "-")         // "hello"
strings.TrimLeft("--hello--", "-")     // "hello--"
strings.TrimRight("--hello--", "-")    // "--hello"
strings.TrimPrefix("hello.go", "hello") // ".go"
strings.TrimSuffix("hello.go", ".go")  // "hello"

strings.Replace(s, "World", "Go", 1)  // replace first
strings.ReplaceAll(s, "l", "L")       // replace all

strings.Split("a,b,c", ",")           // ["a" "b" "c"]
strings.SplitN("a,b,c", ",", 2)       // ["a" "b,c"]
strings.Join([]string{"a","b"}, "-")  // "a-b"

strings.Fields("  a  b  c  ")         // ["a" "b" "c"]

// Builder — efficient string construction
var b strings.Builder
for i := 0; i < 5; i++ {
    fmt.Fprintf(&b, "%d", i)
}
fmt.Println(b.String())  // "01234"

// Reader
r := strings.NewReader("hello")
```

---

## 3. strconv — String Conversions

```go
import "strconv"

// int ↔ string
strconv.Itoa(42)                     // "42"
strconv.Atoi("42")                   // 42, nil
strconv.Atoi("abc")                  // 0, error

// float ↔ string
strconv.FormatFloat(3.14, 'f', 2, 64)  // "3.14"
strconv.ParseFloat("3.14", 64)          // 3.14, nil

// bool ↔ string
strconv.FormatBool(true)               // "true"
strconv.ParseBool("true")              // true, nil

// int with base
strconv.FormatInt(255, 16)             // "ff"
strconv.ParseInt("ff", 16, 64)         // 255, nil
```

---

## 4. os — Operating System

```go
import "os"

// File operations
f, err := os.Open("file.txt")         // read-only
f, err = os.Create("file.txt")        // create or truncate
f, err = os.OpenFile("file.txt", os.O_APPEND|os.O_WRONLY, 0644)
defer f.Close()

// Read/write
data, err := os.ReadFile("file.txt")  // entire file to []byte
err = os.WriteFile("file.txt", data, 0644)

// Directory
os.Mkdir("mydir", 0755)
os.MkdirAll("a/b/c", 0755)
os.Remove("file.txt")
os.RemoveAll("mydir")
os.Rename("old.txt", "new.txt")

// Environment
os.Getenv("HOME")
os.Setenv("KEY", "value")
os.Environ()  // all env vars as []string

// Args & exit
os.Args         // []string of command-line args
os.Exit(1)      // exit with code

// Stdin/Stdout/Stderr
os.Stdout.Write([]byte("hello\n"))
```

---

## 5. io — I/O Primitives

```go
import (
    "io"
    "bytes"
    "strings"
)

// Copy
src := strings.NewReader("hello world")
dst := &bytes.Buffer{}
n, err := io.Copy(dst, src)

// ReadAll
data, err := io.ReadAll(src)

// Discard
io.Copy(io.Discard, largeReader)  // consume but ignore

// Pipe
pr, pw := io.Pipe()
go func() {
    pw.Write([]byte("data"))
    pw.Close()
}()
io.Copy(os.Stdout, pr)

// MultiWriter / TeeReader
mw := io.MultiWriter(os.Stdout, file)  // write to multiple
tr := io.TeeReader(src, log)           // read + copy to log
```

---

## 6. bufio — Buffered I/O

```go
import "bufio"

// Read line by line
f, _ := os.Open("file.txt")
scanner := bufio.NewScanner(f)
for scanner.Scan() {
    fmt.Println(scanner.Text())
}

// Read words
scanner.Split(bufio.ScanWords)
for scanner.Scan() {
    fmt.Println(scanner.Text())
}

// Buffered writer
w := bufio.NewWriter(f)
fmt.Fprintln(w, "hello")
w.Flush()  // must flush!

// Buffered reader
r := bufio.NewReader(f)
line, err := r.ReadString('\n')
```

---

## 7. encoding/json

```go
import "encoding/json"

type Person struct {
    Name    string   `json:"name"`
    Age     int      `json:"age"`
    Tags    []string `json:"tags,omitempty"`
    private string
}

// Marshal (struct → JSON)
p := Person{Name: "Yati", Age: 28, Tags: []string{"go", "dev"}}
data, err := json.Marshal(p)
fmt.Println(string(data))
// {"name":"Yati","age":28,"tags":["go","dev"]}

// Pretty print
data, err = json.MarshalIndent(p, "", "  ")

// Unmarshal (JSON → struct)
jsonStr := `{"name":"Alice","age":25}`
var p2 Person
err = json.Unmarshal([]byte(jsonStr), &p2)

// Stream encoding/decoding (preferred for HTTP)
json.NewEncoder(w).Encode(p)
json.NewDecoder(r.Body).Decode(&p2)

// Dynamic JSON with map
var data map[string]interface{}
json.Unmarshal([]byte(`{"key":"value","num":42}`), &data)
```

---

## 8. net/http — HTTP Client & Server

```go
import "net/http"

// HTTP Server
http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"message": "hello"})
})

http.ListenAndServe(":8080", nil)

// Custom server with timeout
srv := &http.Server{
    Addr:         ":8080",
    Handler:      mux,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    IdleTimeout:  15 * time.Second,
}
srv.ListenAndServe()

// HTTP Client
resp, err := http.Get("https://api.example.com/data")
defer resp.Body.Close()
body, _ := io.ReadAll(resp.Body)

// Client with timeout
client := &http.Client{Timeout: 10 * time.Second}
resp, err = client.Get("https://api.example.com")

// POST with body
payload := map[string]string{"key": "value"}
jsonData, _ := json.Marshal(payload)
resp, err = client.Post("https://api.example.com", "application/json",
    bytes.NewBuffer(jsonData))

// Request with headers
req, _ := http.NewRequest("GET", "https://api.example.com", nil)
req.Header.Set("Authorization", "Bearer token123")
resp, err = client.Do(req)
```

---

## 9. time

```go
import "time"

now := time.Now()
fmt.Println(now.Format("2006-01-02 15:04:05"))  // Go's reference time!

// Parse
t, err := time.Parse("2006-01-02", "2024-01-15")

// Duration
d := 2*time.Hour + 30*time.Minute
time.Sleep(100 * time.Millisecond)

// Timer & Ticker
timer := time.NewTimer(2 * time.Second)
<-timer.C  // blocks until 2s

ticker := time.NewTicker(500 * time.Millisecond)
defer ticker.Stop()
for t := range ticker.C {
    fmt.Println("tick:", t)
}

// Time arithmetic
tomorrow := now.Add(24 * time.Hour)
yesterday := now.AddDate(0, 0, -1)
diff := tomorrow.Sub(now)  // 24h

// Unix timestamp
unix := now.Unix()           // seconds
unixNano := now.UnixNano()   // nanoseconds
fromUnix := time.Unix(unix, 0)
```

---

## 10. sort

```go
import "sort"

// Sort built-in types
ints := []int{5, 2, 4, 1, 3}
sort.Ints(ints)     // [1 2 3 4 5]

strs := []string{"banana", "apple", "cherry"}
sort.Strings(strs)  // [apple banana cherry]

// Custom sort
people := []struct{ Name string; Age int }{
    {"Bob", 25}, {"Alice", 30}, {"Carol", 20},
}
sort.Slice(people, func(i, j int) bool {
    return people[i].Age < people[j].Age
})

// Stable sort (preserves order of equal elements)
sort.SliceStable(people, func(i, j int) bool {
    return people[i].Name < people[j].Name
})

// Check if sorted
sort.IntsAreSorted(ints)

// Binary search
idx, found := sort.Find(len(ints), func(i int) int {
    return ints[i] - 3  // target
})
```

---

## 🎯 Interview Tips

- **Q: What is Go's time format reference time?** → `2006-01-02 15:04:05` — each number is unique (Mon=1, Jan=01/1, 02/2=day, 15=hour, 04=min, 05=sec, 2006=year). It's the actual reference time Go uses.
- **Q: What is the difference between `io.Reader` and `bufio.Reader`?** → `bufio.Reader` wraps an `io.Reader` with an internal buffer, reducing syscalls for small reads.
- **Q: How do you handle large JSON responses?** → Use `json.NewDecoder(resp.Body).Decode(&v)` instead of `json.Unmarshal(io.ReadAll(...))` to avoid loading the entire response into memory.
- **Q: Why should you always close `resp.Body`?** → To release the underlying TCP connection back to the pool. Without it, you leak connections.
- **Q: What is `sort.Slice` vs `sort.Sort`?** → `sort.Slice` takes a less function and is simpler. `sort.Sort` requires implementing the `sort.Interface` (Len, Less, Swap).

---

*Next: [14 — Patterns & Best Practices →](./14_patterns_best_practices.md)*
