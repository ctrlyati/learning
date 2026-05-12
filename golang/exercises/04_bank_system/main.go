package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// Exercise 04 — Mini Bank System
//
// This exercise covers ALL chapters 01–15.
// Complete every section marked with TODO.
// Do not change function signatures, struct definitions, or main().
// =============================================================================

// =============================================================================
// Ch01 — Constants & Types
// =============================================================================

// TODO: define two constants using iota for AccountType
// Savings = 0, Checking = 1
type AccountType int

const (
	// TODO
)

// TODO: define two constants for TransactionType
// Deposit = 0, Withdrawal = 1
type TransactionType int

const (
	// TODO
)

// =============================================================================
// Ch05, Ch06 — Structs
// =============================================================================

type Transaction struct {
	Type      TransactionType
	Amount    float64
	Timestamp time.Time
}

type Account struct {
	ID       string
	Type     AccountType
	Owner    string
	balance  float64 // unexported — access via methods only
	mu       sync.Mutex
	history  []Transaction
}

// =============================================================================
// Ch07 — Interface
// =============================================================================

// TODO: define a BankAccount interface with these methods:
// Deposit(amount float64) error
// Withdraw(amount float64) error
// Balance() float64
// History() []Transaction
// GetID() string
type BankAccount interface {
	// TODO
}

// =============================================================================
// Ch08 — Custom Errors
// =============================================================================

// TODO: declare these sentinel errors using errors.New:
// ErrInsufficientFunds
// ErrInvalidAmount   (amount <= 0)
// ErrAccountNotFound
var (
	// TODO
)

// =============================================================================
// Ch06 — Methods (implement BankAccount interface on *Account)
// =============================================================================

// TODO: implement Deposit
// - return ErrInvalidAmount if amount <= 0
// - add to balance
// - append a Transaction to history
// - must be safe for concurrent use
func (a *Account) Deposit(amount float64) error {
	return nil
}

// TODO: implement Withdraw
// - return ErrInvalidAmount if amount <= 0
// - return ErrInsufficientFunds if balance < amount
// - subtract from balance
// - append a Transaction to history
// - must be safe for concurrent use
func (a *Account) Withdraw(amount float64) error {
	return nil
}

// TODO: implement Balance — returns current balance (concurrent safe)
func (a *Account) Balance() float64 {
	return 0
}

// TODO: implement History — returns a copy of the transaction history
// return a copy, not the original slice
func (a *Account) History() []Transaction {
	return nil
}

// TODO: implement GetID — returns the account ID
func (a *Account) GetID() string {
	return ""
}

// =============================================================================
// Ch05, Ch09 — Bank (manages accounts, concurrent safe)
// =============================================================================

type Bank struct {
	mu       sync.RWMutex
	accounts map[string]BankAccount
	notify   chan Transaction // ch10 — notifications channel
}

// TODO: implement NewBank — returns a *Bank with initialized map and a buffered notify channel (size 100)
func NewBank() *Bank {
	return nil
}

// TODO: implement CreateAccount — creates a new *Account and stores it in the map
// return ErrInvalidAmount if initialDeposit < 0
// assign a unique ID using fmt.Sprintf("ACC-%d", time.Now().UnixNano())
func (b *Bank) CreateAccount(owner string, accType AccountType, initialDeposit float64) (BankAccount, error) {
	return nil, nil
}

// TODO: implement GetAccount — returns the account or ErrAccountNotFound
func (b *Bank) GetAccount(id string) (BankAccount, error) {
	return nil, nil
}

// TODO: implement Transfer — transfers amount from one account to another
// wrap any errors with fmt.Errorf("transfer: %w", err)
func (b *Bank) Transfer(fromID, toID string, amount float64) error {
	return nil
}

// =============================================================================
// Ch10 — Notification Worker (channel consumer)
// =============================================================================

// TODO: implement StartNotificationWorker
// - run a goroutine that reads from b.notify channel until ctx is cancelled
// - for each transaction print: "NOTIFY: [Deposit/Withdrawal] amount=X at timestamp"
// - when ctx is done, drain remaining notifications then return
func (b *Bank) StartNotificationWorker(ctx context.Context) {
}

// =============================================================================
// Ch03 — Functional Options Pattern (Ch14 — Patterns)
// =============================================================================

type ServerOption func(*http.Server)

// TODO: implement WithTimeout — returns a ServerOption that sets ReadTimeout and WriteTimeout
func WithTimeout(d time.Duration) ServerOption {
	return nil
}

// TODO: implement WithAddr — returns a ServerOption that sets the Addr
func WithAddr(addr string) ServerOption {
	return nil
}

// =============================================================================
// Ch13 — HTTP Handlers
// =============================================================================

type Handler struct {
	bank *Bank
}

// TODO: implement createAccountHandler
// POST /accounts
// body: {"owner": "Yati", "type": 0, "initial_deposit": 1000}
// respond 201 with {"id": "ACC-...", "owner": "...", "balance": 1000}
func (h *Handler) createAccountHandler(w http.ResponseWriter, r *http.Request) {
}

// TODO: implement getBalanceHandler
// GET /accounts/{id}/balance
// respond 200 with {"id": "...", "balance": ...}
// respond 404 if not found
func (h *Handler) getBalanceHandler(w http.ResponseWriter, r *http.Request) {
}

// TODO: implement depositHandler
// POST /accounts/{id}/deposit
// body: {"amount": 500}
// respond 200 with updated balance
// respond 400 on invalid amount
// respond 404 if not found
func (h *Handler) depositHandler(w http.ResponseWriter, r *http.Request) {
}

// TODO: implement transferHandler
// POST /transfer
// body: {"from": "ACC-1", "to": "ACC-2", "amount": 100}
// respond 200 on success
// respond 400 on error (wrap the error message in JSON)
func (h *Handler) transferHandler(w http.ResponseWriter, r *http.Request) {
}

// TODO: implement historyHandler
// GET /accounts/{id}/history
// respond 200 with JSON array of transactions
// respond 404 if not found
func (h *Handler) historyHandler(w http.ResponseWriter, r *http.Request) {
}

// helper — already implemented
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// helper — extracts {id} from path like /accounts/ACC-123/balance
func extractID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// =============================================================================
// Ch09, Ch10 — Worker Pool (process deposits concurrently)
// =============================================================================

type DepositJob struct {
	AccountID string
	Amount    float64
	Result    chan error
}

// TODO: implement StartWorkerPool
// - spawn `n` worker goroutines
// - each worker reads jobs from the jobs channel
// - for each job: call bank.GetAccount, then Deposit, send result to job.Result
// - workers stop when jobs channel is closed OR ctx is done
func StartWorkerPool(ctx context.Context, bank *Bank, jobs <-chan DepositJob, n int) {
}

// =============================================================================
// main — do not modify
// =============================================================================

func main() {
	bank := NewBank()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start notification worker
	bank.StartNotificationWorker(ctx)

	// start worker pool for deposits
	jobs := make(chan DepositJob, 50)
	StartWorkerPool(ctx, bank, jobs, 3)

	// wire up HTTP handlers
	h := &Handler{bank: bank}
	mux := http.NewServeMux()
	mux.HandleFunc("/accounts", h.createAccountHandler)
	mux.HandleFunc("/accounts/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/balance"):
			h.getBalanceHandler(w, r)
		case strings.HasSuffix(path, "/deposit"):
			h.depositHandler(w, r)
		case strings.HasSuffix(path, "/history"):
			h.historyHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/transfer", h.transferHandler)

	// build server with options pattern
	srv := buildServer(mux,
		WithAddr(":8080"),
		WithTimeout(10*time.Second),
	)

	fmt.Println("bank server running on :8080")
	srv.ListenAndServe()
}

// buildServer — already implemented
func buildServer(mux http.Handler, opts ...ServerOption) *http.Server {
	srv := &http.Server{Handler: mux}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// =============================================================================
// Test with curl:
//
// Create account:
// curl -X POST http://localhost:8080/accounts \
//   -d '{"owner":"Yati","type":0,"initial_deposit":1000}'
//
// Get balance:
// curl http://localhost:8080/accounts/ACC-{id}/balance
//
// Deposit:
// curl -X POST http://localhost:8080/accounts/ACC-{id}/deposit \
//   -d '{"amount":500}'
//
// Transfer:
// curl -X POST http://localhost:8080/transfer \
//   -d '{"from":"ACC-1","to":"ACC-2","amount":200}'
//
// History:
// curl http://localhost:8080/accounts/ACC-{id}/history
// =============================================================================

// =============================================================================
// Chapters covered:
// Ch01 — iota constants (AccountType, TransactionType)
// Ch02 — control flow in handlers and workers
// Ch03 — variadic options pattern (ServerOption)
// Ch04 — pointer receivers on Account methods
// Ch05 — maps (accounts), slices (history, transactions)
// Ch06 — structs and methods (Account, Bank, Handler)
// Ch07 — BankAccount interface
// Ch08 — custom sentinel errors, error wrapping with %w
// Ch09 — goroutines (worker pool, notification worker)
// Ch10 — channels (notify chan, jobs chan, result chan)
// Ch11 — single package, clean separation of concerns
// Ch12 — code is testable (interfaces, dependency injection)
// Ch13 — HTTP handlers, JSON encode/decode, time.Now()
// Ch14 — functional options pattern, worker pool pattern
// Ch15 — interview-ready: mutex, channels, interfaces, error handling
// =============================================================================
