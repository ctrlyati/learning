package main

import "fmt"

// Exercise 01 — Todo List
//
// Complete the missing code marked with TODO.
// Do not change the main() function or the struct definition.
//
// Requirements:
// - Add a todo item
// - Complete a todo item (mark as done)
// - List all todos
// - Delete a todo by index
// - Count how many are not done yet

type Todo struct {
	Title string
	Done  bool
}

type TodoList struct {
	// TODO: add a field to store a slice of Todo
}

// TODO: implement Add — adds a new Todo with the given title (Done = false)
func (t *TodoList) Add(title string) {

}

// TODO: implement Complete — marks the todo at the given index as Done = true
// if index is out of range, do nothing
func (t *TodoList) Complete(index int) {

}

// TODO: implement Delete — removes the todo at the given index
// if index is out of range, do nothing
func (t *TodoList) Delete(index int) {

}

// TODO: implement Remaining — returns how many todos are not done yet
func (t *TodoList) Remaining() int {
	return 0
}

// TODO: implement List — prints all todos with their index, title, and status
// format: "[0] Buy groceries (pending)" or "[1] Walk dog (done)"
func (t *TodoList) List() {

}

func main() {
	list := &TodoList{}

	list.Add("Buy groceries")
	list.Add("Walk dog")
	list.Add("Read Go chapter 06")
	list.Add("Write exercises")

	list.List()
	fmt.Println("Remaining:", list.Remaining())

	fmt.Println("\n--- completing index 1 ---")
	list.Complete(1)
	list.List()
	fmt.Println("Remaining:", list.Remaining())

	fmt.Println("\n--- deleting index 0 ---")
	list.Delete(0)
	list.List()
	fmt.Println("Remaining:", list.Remaining())
}

// Expected output:
// [0] Buy groceries (pending)
// [1] Walk dog (pending)
// [2] Read Go chapter 06 (pending)
// [3] Write exercises (pending)
// Remaining: 4
//
// --- completing index 1 ---
// [0] Buy groceries (pending)
// [1] Walk dog (done)
// [2] Read Go chapter 06 (pending)
// [3] Write exercises (pending)
// Remaining: 3
//
// --- deleting index 0 ---
// [0] Walk dog (done)
// [1] Read Go chapter 06 (pending)
// [2] Write exercises (pending)
// Remaining: 2
