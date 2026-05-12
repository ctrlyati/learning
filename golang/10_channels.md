# 10 — Channels

> **Goal:** Master Go channels — the primary way goroutines communicate and synchronize.

---

## 1. What is a Channel?

A channel is a typed conduit for sending and receiving values between goroutines:

```go
// Create
ch := make(chan int)         // unbuffered
ch2 := make(chan string, 5)  // buffered with capacity 5

// Send (blocks until receiver is ready on unbuffered)
ch <- 42

// Receive (blocks until sender sends)
val := <-ch

// Receive with ok check (false means channel closed)
val, ok := <-ch
```

---

## 2. Unbuffered Channels (Synchronous)

```go
func main() {
    ch := make(chan string)

    go func() {
        ch <- "hello from goroutine"  // blocks until main receives
    }()

    msg := <-ch  // blocks until goroutine sends
    fmt.Println(msg)
}
```

Unbuffered channels provide a **synchronization point** — sender blocks until receiver is ready, and vice versa.

---

## 3. Buffered Channels (Asynchronous)

```go
ch := make(chan int, 3)  // buffer of 3

ch <- 1  // doesn't block (buffer not full)
ch <- 2
ch <- 3
// ch <- 4  // would block (buffer full)

fmt.Println(<-ch)  // 1
fmt.Println(<-ch)  // 2
fmt.Println(<-ch)  // 3
```

---

## 4. Closing Channels

```go
ch := make(chan int, 3)
ch <- 1
ch <- 2
ch <- 3
close(ch)  // signal: no more values will be sent

// Receive until closed
for v := range ch {
    fmt.Println(v)  // 1, 2, 3
}

// After closed, receives return zero value + false
v, ok := <-ch
fmt.Println(v, ok)  // 0, false
```

**Rules:**
- Only the **sender** should close a channel
- Sending to a closed channel **panics**
- Closing an already closed channel **panics**
- Receiving from a closed channel returns zero value immediately

---

## 5. Directional Channels

```go
// Send-only channel
func producer(ch chan<- int) {
    ch <- 42
    // <-ch  // COMPILE ERROR — can't receive on send-only
}

// Receive-only channel
func consumer(ch <-chan int) {
    v := <-ch
    fmt.Println(v)
    // ch <- 1  // COMPILE ERROR — can't send on receive-only
}

func main() {
    ch := make(chan int, 1)
    go producer(ch)
    consumer(ch)
}
```

---

## 6. select Statement

`select` lets a goroutine wait on multiple channel operations:

```go
ch1 := make(chan string, 1)
ch2 := make(chan string, 1)

ch1 <- "one"
ch2 <- "two"

// select picks one ready case at random
select {
case msg1 := <-ch1:
    fmt.Println("received from ch1:", msg1)
case msg2 := <-ch2:
    fmt.Println("received from ch2:", msg2)
}
```

### select with default (non-blocking)

```go
ch := make(chan int)

select {
case v := <-ch:
    fmt.Println("received:", v)
default:
    fmt.Println("no value ready")  // executes immediately if ch is empty
}
```

### select with timeout

```go
ch := make(chan string)

select {
case msg := <-ch:
    fmt.Println("received:", msg)
case <-time.After(2 * time.Second):
    fmt.Println("timeout!")
}
```

---

## 7. Done Channel Pattern

```go
func work(done <-chan struct{}) {
    for {
        select {
        case <-done:
            fmt.Println("stopping worker")
            return
        default:
            fmt.Println("working...")
            time.Sleep(100 * time.Millisecond)
        }
    }
}

func main() {
    done := make(chan struct{})
    go work(done)

    time.Sleep(300 * time.Millisecond)
    close(done)  // signal all listeners to stop
    time.Sleep(50 * time.Millisecond)
}
```

---

## 8. Pipeline Pattern

Chain goroutines together to process data:

```go
// Stage 1: generate numbers
func generate(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            out <- n
        }
    }()
    return out
}

// Stage 2: square numbers
func square(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            out <- n * n
        }
    }()
    return out
}

func main() {
    // Pipeline: generate → square → print
    c := generate(2, 3, 4, 5)
    out := square(c)

    for v := range out {
        fmt.Println(v)  // 4, 9, 16, 25
    }
}
```

---

## 9. Fan-Out / Fan-In Pattern

```go
// Fan-out: distribute work to multiple goroutines
func fanOut(in <-chan int, workers int) []<-chan int {
    channels := make([]<-chan int, workers)
    for i := 0; i < workers; i++ {
        channels[i] = square(in)  // each worker reads from same input
    }
    return channels
}

// Fan-in: merge multiple channels into one
func merge(channels ...<-chan int) <-chan int {
    var wg sync.WaitGroup
    merged := make(chan int)

    output := func(c <-chan int) {
        defer wg.Done()
        for v := range c {
            merged <- v
        }
    }

    wg.Add(len(channels))
    for _, c := range channels {
        go output(c)
    }

    go func() {
        wg.Wait()
        close(merged)
    }()

    return merged
}
```

---

## 10. Semaphore Pattern (limit concurrency)

```go
// Allow max 3 concurrent goroutines
sem := make(chan struct{}, 3)

var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        sem <- struct{}{}        // acquire
        defer func() { <-sem }() // release
        
        fmt.Printf("worker %d running\n", id)
        time.Sleep(100 * time.Millisecond)
    }(i)
}
wg.Wait()
```

---

## 🎯 Interview Tips

- **Q: What is the difference between buffered and unbuffered channels?** → Unbuffered: sender blocks until receiver is ready (synchronous handoff). Buffered: sender blocks only when buffer is full (asynchronous up to capacity).
- **Q: What happens when you send to a closed channel?** → Panic.
- **Q: What happens when you receive from a closed channel?** → Returns zero value immediately (and `false` for the ok check).
- **Q: Who should close a channel?** → The sender. Never the receiver. Closing signals "no more values."
- **Q: What is the select statement?** → Like a switch for channel operations. Picks one ready case at random. Executes `default` if none are ready.
- **Q: How do you implement a timeout with channels?** → `select` with `time.After(duration)`.
- **Q: What is a done channel?** → A channel (often `chan struct{}`) used to signal goroutines to stop. Closing it broadcasts to all receivers simultaneously.
- **Q: Channels vs Mutexes — when to use which?** → Channels: passing ownership of data, coordinating goroutines, signaling. Mutex: protecting shared state.

---

*Next: [11 — Packages & Modules →](./11_packages_modules.md)*
