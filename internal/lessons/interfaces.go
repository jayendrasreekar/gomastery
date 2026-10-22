package lessons

import (
	"errors"
	"fmt"
	"strings"
)

// =============================================================================
// LESSON: IMPLICIT INTERFACES
// =============================================================================
//
// Go interfaces are IMPLICIT (structural typing, "duck typing").
// A type satisfies an interface simply by implementing its methods.
// There is NO `implements` keyword. This is one of Go's most powerful features:
//   - Decouples definition from implementation
//   - Allows interfaces to be defined by the CONSUMER, not the provider
//   - Makes testing trivial (any mock that has the methods works)
//
// Best practice: "Accept interfaces, return structs."

// Speaker is a small, focused interface — the Go way.
// WHY small interfaces? They're easier to satisfy, compose, and mock.
// The standard library's io.Reader (1 method) is the gold standard.
type Speaker interface {
	Speak() string
}

// Dog satisfies Speaker implicitly — no "implements" declaration needed.
type Dog struct {
	Name string
}

// Speak makes Dog satisfy the Speaker interface.
func (d Dog) Speak() string {
	return fmt.Sprintf("Woof! I'm %s", d.Name)
}

// Robot also satisfies Speaker, completely independently of Dog.
type Robot struct {
	Model string
}

// Speak makes Robot satisfy the Speaker interface.
func (r Robot) Speak() string {
	return fmt.Sprintf("Beep boop. Unit %s online.", r.Model)
}

// ImplicitInterfaces demonstrates Go's structural interface satisfaction.
func ImplicitInterfaces() {
	fmt.Println("━━━ Implicit Interfaces ━━━")

	// Both Dog and Robot satisfy Speaker without declaring it.
	// WHY: The compiler checks method sets at compile time. If a type has
	// all the methods an interface requires, it satisfies it. Period.
	speakers := []Speaker{
		Dog{Name: "Rex"},
		Robot{Model: "T-800"},
	}

	for _, s := range speakers {
		// Polymorphism! We don't know the concrete type, only that it can Speak().
		fmt.Printf("  %T says: %s\n", s, s.Speak())
	}

	fmt.Println("\n  💡 Key insight: Interfaces are defined by the CONSUMER.")
	fmt.Println("     You can write an interface for types you don't own.")
}

// =============================================================================
// LESSON: STRUCT EMBEDDING (Composition over Inheritance)
// =============================================================================
//
// Go has NO inheritance. Instead, it uses COMPOSITION via struct embedding.
// When you embed a struct, its methods are "promoted" to the outer struct.
// This gives you code reuse without the fragile base class problem.

// Base provides shared functionality.
type Base struct {
	ID string
}

// Describe is a method on Base that will be promoted to any embedding struct.
func (b Base) Describe() string {
	return fmt.Sprintf("Base entity with ID: %s", b.ID)
}

// Greet is another Base method that Derived will override.
func (b Base) Greet() string {
	return "Hello from Base!"
}

// Derived embeds Base — it gets all of Base's methods automatically.
// This is NOT inheritance; it's composition. Derived HAS a Base.
type Derived struct {
	Base  // Embedded (anonymous) field — methods are promoted
	Extra string
}

// Greet OVERRIDES the promoted Base.Greet method.
// WHY: The outer struct's method takes precedence over the embedded one.
// You can still access the original via d.Base.Greet().
func (d Derived) Greet() string {
	return fmt.Sprintf("Hello from Derived! Extra: %s (Base said: %s)", d.Extra, d.Base.Greet())
}

// StructEmbedding demonstrates composition via embedding.
func StructEmbedding() {
	fmt.Println("\n━━━ Struct Embedding (Composition) ━━━")

	d := Derived{
		Base:  Base{ID: "abc-123"},
		Extra: "premium",
	}

	// Describe() is promoted from Base — called directly on Derived
	fmt.Printf("  d.Describe() = %s\n", d.Describe())

	// Greet() is overridden by Derived
	fmt.Printf("  d.Greet()    = %s\n", d.Greet())

	// You can still access the embedded Base's original method:
	fmt.Printf("  d.Base.Greet() = %s\n", d.Base.Greet())

	fmt.Println("\n  💡 Composition > Inheritance:")
	fmt.Println("     No fragile base class problem. Explicit. Testable.")
}

// =============================================================================
// LESSON: GENERICS (Go 1.18+)
// =============================================================================
//
// Generics let you write functions and types that work with ANY type,
// while keeping compile-time type safety. Before generics, you'd use
// interface{} (now `any`) and lose type checking.
//
// Key concepts:
//   - Type parameters: [T any], [T comparable], [T constraints.Ordered]
//   - Type constraints: interfaces that restrict what T can be
//   - Type inference: compiler can often infer T from arguments

// Filter returns a new slice containing only elements where predicate returns true.
// WHY generic: Without generics, you'd need FilterInts, FilterStrings, etc.
// The type parameter [T any] means T can be ANY type.
func Filter[T any](slice []T, predicate func(T) bool) []T {
	// Pre-allocate with len=0 but reasonable capacity to reduce allocations.
	// WHY: append() doubles capacity when needed, but starting at 0 means
	// the first append always allocates. If we expect roughly half to match,
	// we could use len(slice)/2, but 0 is simpler and correct.
	result := make([]T, 0)
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// Map transforms each element of a slice using the given function.
// WHY two type params: Input type T may differ from output type U.
// Example: Map([]string, func(string) int) → []int
func Map[T any, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice)) // Pre-allocate exact size — we know the output length
	for i, v := range slice {
		result[i] = transform(v)
	}
	return result
}

// GenericsDemo demonstrates generic Filter and Map functions.
func GenericsDemo() {
	fmt.Println("\n━━━ Generics (Filter & Map) ━━━")

	// --- Filter: Keep only even numbers ---
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	evens := Filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Printf("  Filter evens from %v → %v\n", nums, evens)

	// --- Filter: Keep only long strings ---
	words := []string{"Go", "is", "awesome", "and", "powerful"}
	long := Filter(words, func(s string) bool { return len(s) > 3 })
	fmt.Printf("  Filter long words from %v → %v\n", words, long)

	// --- Map: Square each number ---
	squares := Map(nums, func(n int) int { return n * n })
	fmt.Printf("  Map square %v → %v\n", nums, squares)

	// --- Map: Transform type (string → int length) ---
	lengths := Map(words, func(s string) int { return len(s) })
	fmt.Printf("  Map string→length %v → %v\n", words, lengths)

	// --- Chaining: Filter then Map ---
	result := Map(Filter(nums, func(n int) bool { return n > 5 }), func(n int) string {
		return fmt.Sprintf("%d²=%d", n, n*n)
	})
	fmt.Printf("  Chain Filter(>5) → Map(format): %v\n", result)
}

// =============================================================================
// LESSON: ERROR CHAINS (errors.Is, errors.As, fmt.Errorf %w)
// =============================================================================
//
// Go 1.13+ introduced error wrapping with fmt.Errorf("%w", err).
// This creates an error CHAIN that you can inspect with:
//   - errors.Is(err, target): walks the chain looking for a specific error VALUE
//   - errors.As(err, &target): walks the chain looking for a specific error TYPE
//   - errors.Unwrap(err): gets the next error in the chain
//
// WHY: This replaces the old pattern of comparing error strings. You can add
// context at each layer while preserving the original error for inspection.

// ErrNotFound is a sentinel error — a package-level error value.
// WHY sentinel: Callers can compare against it with errors.Is().
// Convention: name them Err<Something>.
var ErrNotFound = errors.New("not found")

// AppError is a custom error type with structured data.
// WHY custom type: When you need more than a string — error codes, HTTP status, etc.
// It implements the error interface (has Error() string) and Unwrap() for chaining.
type AppError struct {
	Code    int
	Message string
	Err     error // The wrapped (underlying) error
}

// Error satisfies the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error, enabling errors.Is/As to walk the chain.
// WHY: Without Unwrap(), errors.Is/As can't see through your custom error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// findUser simulates a repository lookup that returns a wrapped error.
func findUser(id string) (*User, error) {
	if id == "missing" {
		// Wrap ErrNotFound with context using %w verb.
		// The resulting error chain: AppError → ErrNotFound
		return nil, &AppError{
			Code:    404,
			Message: fmt.Sprintf("user %q not found in database", id),
			Err:     ErrNotFound, // This is what errors.Is will find
		}
	}
	return &User{Name: "Found", Age: 25}, nil
}

// ErrorChainsDemo demonstrates error wrapping, Is, and As.
func ErrorChainsDemo() {
	fmt.Println("\n━━━ Error Chains (errors.Is / errors.As) ━━━")

	_, err := findUser("missing")
	if err != nil {
		fmt.Printf("  Error: %v\n", err)

		// errors.Is: Check if ANY error in the chain matches a sentinel value.
		// It walks the Unwrap() chain automatically.
		if errors.Is(err, ErrNotFound) {
			fmt.Println("  ✅ errors.Is(err, ErrNotFound) = true")
			fmt.Println("     → We can handle 'not found' specifically (e.g., return 404)")
		}

		// errors.As: Extract a specific error TYPE from the chain.
		// It walks the chain and assigns the first match to the target pointer.
		var appErr *AppError
		if errors.As(err, &appErr) {
			fmt.Printf("  ✅ errors.As found AppError: code=%d, msg=%s\n", appErr.Code, appErr.Message)
			fmt.Println("     → We can access structured error data (code, message)")
		}
	}

	// Multi-layer wrapping with fmt.Errorf %w
	fmt.Println("\n  Multi-layer wrapping:")
	baseErr := ErrNotFound
	layer1 := fmt.Errorf("repository: %w", baseErr)
	layer2 := fmt.Errorf("service: %w", layer1)
	layer3 := fmt.Errorf("handler: %w", layer2)

	fmt.Printf("  Full chain: %v\n", layer3)
	fmt.Printf("  errors.Is(layer3, ErrNotFound) = %v ← walks through 3 layers!\n", errors.Is(layer3, ErrNotFound))

	// Show the chain visually
	fmt.Println("\n  Error chain visualization:")
	var current error = layer3
	depth := 0
	for current != nil {
		indent := strings.Repeat("  ", depth)
		fmt.Printf("    %s→ %s\n", indent, current.Error())
		current = errors.Unwrap(current)
		depth++
	}
}

// RunInterfacesLesson is an alias for RunAllInterfaceLessons to match the entrypoint runner.
func RunInterfacesLesson() {
	RunAllInterfaceLessons()
}

// RunAllInterfaceLessons executes all interface-related lessons in sequence.
func RunAllInterfaceLessons() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║   📚 LESSON: Interfaces, Generics & Errors   ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	ImplicitInterfaces()
	StructEmbedding()
	GenericsDemo()
	ErrorChainsDemo()
}
