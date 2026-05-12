package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Exercise 03 — Build a simple in-memory product API
//
// Complete the missing handlers and helper functions marked with TODO.
// The routes are already registered in main() — do not change them.
//
// Routes:
// GET    /products         → list all products
// GET    /products/{id}    → get one product by id
// POST   /products         → create a new product
// DELETE /products/{id}    → delete a product

type Product struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

// in-memory store
var (
	products = []Product{}
	nextID   = 1
)

var ErrNotFound = errors.New("product not found")

// TODO: implement findByID — returns the product and its index, or ErrNotFound
func findByID(id int) (Product, int, error) {
	return Product{}, -1, nil
}

// TODO: implement listHandler
// GET /products → respond with JSON array of all products
func listHandler(w http.ResponseWriter, r *http.Request) {

}

// TODO: implement getHandler
// GET /products/{id} → respond with the product or 404 if not found
func getHandler(w http.ResponseWriter, r *http.Request, id int) {

}

// TODO: implement createHandler
// POST /products → decode body into Product, assign next ID, append to list, respond 201 with created product
func createHandler(w http.ResponseWriter, r *http.Request) {

}

// TODO: implement deleteHandler
// DELETE /products/{id} → delete the product or 404 if not found, respond 204
func deleteHandler(w http.ResponseWriter, r *http.Request, id int) {

}

// helper — write JSON response
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// router — already implemented, do not change
func router(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")

	if path == "/products" {
		switch r.Method {
		case http.MethodGet:
			listHandler(w, r)
		case http.MethodPost:
			createHandler(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasPrefix(path, "/products/") {
		idStr := strings.TrimPrefix(path, "/products/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			getHandler(w, r, id)
		case http.MethodDelete:
			deleteHandler(w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func main() {
	http.HandleFunc("/products", router)
	http.HandleFunc("/products/", router)

	fmt.Println("server running on :8080")
	http.ListenAndServe(":8080", nil)
}

// Test with curl:
//
// Create products:
// curl -X POST http://localhost:8080/products -d '{"name":"Keyboard","price":49.99}'
// curl -X POST http://localhost:8080/products -d '{"name":"Mouse","price":29.99}'
//
// List all:
// curl http://localhost:8080/products
//
// Get one:
// curl http://localhost:8080/products/1
//
// Delete:
// curl -X DELETE http://localhost:8080/products/1
//
// List again (should be missing id 1):
// curl http://localhost:8080/products
