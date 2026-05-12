package main

import (
	"fmt"
	"sync"
)

// Exercise 02 — Fix the Bugs
//
// There are 6 bugs in this file. Find and fix all of them.
// Each bug is marked with a "BUG:" comment explaining what should happen.
// Do not change the expected output at the bottom.

// ----- Bug 1 -----
// BUG: this should double the value at the pointer, but it doesn't
func double(n int) {
	n *= 2
}

// ----- Bug 2 -----
// BUG: this should return the sum of all numbers, but it always returns 0
func sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total = n
	}
	return total
}

// ----- Bug 3 -----
// BUG: this closure should print 0, 1, 2 but prints wrong values
func makeFuncs() []func() int {
	funcs := make([]func() int, 3)
	for i := 0; i < 3; i++ {
		funcs[i] = func() int { return i }
	}
	return funcs
}

// ----- Bug 4 -----
// BUG: this should safely increment counter from multiple goroutines
// but has a race condition
type Counter struct {
	mu    sync.Mutex
	count int
}

func (c *Counter) Increment() {
	c.count++
}

func (c *Counter) Value() int {
	return c.count
}

// ----- Bug 5 -----
// BUG: this should return error when name is empty, but it doesn't compile
func greet(name string) (string, error) {
	if name == "" {
		return "name cannot be empty"
	}
	return "Hello, " + name + "!", nil
}

// ----- Bug 6 -----
// BUG: this map operation will panic — fix it
func wordCount(words []string) map[string]int {
	var counts map[string]int
	for _, w := range words {
		counts[w]++
	}
	return counts
}

func main() {
	// Bug 1
	x := 10
	double(&x)
	fmt.Println("doubled:", x) // expected: 20

	// Bug 2
	fmt.Println("sum:", sum(1, 2, 3, 4, 5)) // expected: 15

	// Bug 3
	funcs := makeFuncs()
	fmt.Println("funcs:", funcs[0](), funcs[1](), funcs[2]()) // expected: 0 1 2

	// Bug 4
	var wg sync.WaitGroup
	c := &Counter{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Increment()
		}()
	}
	wg.Wait()
	fmt.Println("counter:", c.Value()) // expected: 100

	// Bug 5
	msg, err := greet("")
	fmt.Println("greet err:", err) // expected: name cannot be empty
	msg, err = greet("Yati")
	fmt.Println("greet msg:", msg) // expected: Hello, Yati!
	_ = err

	// Bug 6
	counts := wordCount([]string{"go", "is", "go", "fun"})
	fmt.Println("wordcount:", counts) // expected: map[fun:1 go:2 is:1]
}
