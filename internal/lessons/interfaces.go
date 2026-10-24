package lessons

import (
	"errors"
	"fmt"
	"strings"
)

// =============================================================================
// LESSON: IMPLICIT INTERFACES
// =============================================================================

type Speaker interface {
	Speak() string
}

type Dog struct {
	Name string
}

func (d Dog) Speak() string {
	return fmt.Sprintf("Woof! I'm %s", d.Name)
}

type Robot struct {
	Model string
}

func (r Robot) Speak() string {
	return fmt.Sprintf("Beep boop. Unit %s online.", r.Model)
}

func ImplicitInterfaces() {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 1. IMPLICIT INTERFACES (DUCK TYPING)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ type Speaker interface { Speak() string }         │")
	fmt.Println("  │ type Dog struct { Name string }                   │")
	fmt.Println("  │ func (d Dog) Speak() string { ... }               │")
	fmt.Println("  │ // Notice: No 'implements' keyword exists in Go!  │")
	fmt.Println("  │ speakers := []Speaker{Dog{\"Rex\"}, Robot{\"T-800\"}} │")
	fmt.Println("  └───────────────────────────────────────────────────┘")

	speakers := []Speaker{
		Dog{Name: "Rex"},
		Robot{Model: "T-800"},
	}

	for _, s := range speakers {
		fmt.Printf("  👉 Polymorphism: %T says: %q\n", s, s.Speak())
	}
	fmt.Println("  💡 Insight: Interfaces in Go are defined by the consumer, not provider.")
}

// =============================================================================
// LESSON: STRUCT EMBEDDING (Composition over Inheritance)
// =============================================================================

type Base struct {
	ID string
}

func (b Base) Describe() string {
	return fmt.Sprintf("Base entity with ID: %s", b.ID)
}

func (b Base) Greet() string {
	return "Hello from Base!"
}

type Derived struct {
	Base  // Embedded anonymous struct (methods promoted!)
	Extra string
}

func (d Derived) Greet() string {
	return fmt.Sprintf("Hello from Derived! (Base said: %s)", d.Base.Greet())
}

func StructEmbedding() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 2. STRUCT EMBEDDING (COMPOSITION)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ type Derived struct {                             │")
	fmt.Println("  │     Base   // Promotes Describe() and Greet()     │")
	fmt.Println("  │     Extra string                                  │")
	fmt.Println("  │ }                                                 │")
	fmt.Println("  │ func (d Derived) Greet() string { ... } // Override│")
	fmt.Println("  └───────────────────────────────────────────────────┘")

	d := Derived{
		Base:  Base{ID: "abc-123"},
		Extra: "premium",
	}

	fmt.Printf("  👉 Promoted method: d.Describe()   = %s\n", d.Describe())
	fmt.Printf("  👉 Overridden method: d.Greet()    = %s\n", d.Greet())
	fmt.Printf("  👉 Explicit parent: d.Base.Greet() = %s\n", d.Base.Greet())
}

// =============================================================================
// LESSON: GENERICS (Go 1.18+)
// =============================================================================

func Filter[T any](slice []T, predicate func(T) bool) []T {
	result := make([]T, 0)
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

func Map[T any, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice))
	for i, v := range slice {
		result[i] = transform(v)
	}
	return result
}

func GenericsDemo() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 3. GENERICS (Filter & Map)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ func Filter[T any](slice []T, fn func(T) bool) []T│")
	fmt.Println("  │ func Map[T any, U any](slice []T, fn ...) []U     │")
	fmt.Println("  │ evens := Filter(nums, func(n int) bool { ... })   │")
	fmt.Println("  └───────────────────────────────────────────────────┘")

	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	evens := Filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Printf("  👉 Filter evens: %v → %v\n", nums, evens)

	words := []string{"Go", "is", "awesome", "and", "powerful"}
	long := Filter(words, func(s string) bool { return len(s) > 3 })
	fmt.Printf("  👉 Filter words: %v → %v\n", words, long)

	squares := Map(nums, func(n int) int { return n * n })
	fmt.Printf("  👉 Map squares:  %v → %v\n", nums, squares)
}

// =============================================================================
// LESSON: ERROR CHAINS
// =============================================================================

var ErrNotFound = errors.New("not found")

type AppError struct {
	Code    int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func findUser(id string) (*Dog, error) {
	if id == "missing" {
		return nil, &AppError{
			Code:    404,
			Message: fmt.Sprintf("entity %q not found", id),
			Err:     ErrNotFound,
		}
	}
	return &Dog{Name: "Found"}, nil
}

func ErrorChainsDemo() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 4. ERROR WRAPPING & CHAINS (errors.Is & errors.As)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ var ErrNotFound = errors.New(\"not found\")          │")
	fmt.Println("  │ layer1 := fmt.Errorf(\"repository: %w\", ErrNotFound)│")
	fmt.Println("  │ layer2 := fmt.Errorf(\"service: %w\", layer1)       │")
	fmt.Println("  │ errors.Is(layer2, ErrNotFound) // true!           │")
	fmt.Println("  └───────────────────────────────────────────────────┘")

	_, err := findUser("missing")
	if err != nil {
		fmt.Printf("  👉 Caught error: %v\n", err)
		if errors.Is(err, ErrNotFound) {
			fmt.Println("  ✅ errors.Is(err, ErrNotFound) == true (found sentinel through wrappers)")
		}

		var appErr *AppError
		if errors.As(err, &appErr) {
			fmt.Printf("  ✅ errors.As found AppError: Code=%d, Message=%q\n", appErr.Code, appErr.Message)
		}
	}

	baseErr := ErrNotFound
	layer1 := fmt.Errorf("repository: %w", baseErr)
	layer2 := fmt.Errorf("service: %w", layer1)
	layer3 := fmt.Errorf("handler: %w", layer2)

	fmt.Printf("\n  👉 Deep chain: %v\n", layer3)
	fmt.Println("  👉 Visualization:")
	var current error = layer3
	depth := 0
	for current != nil {
		indent := strings.Repeat("  ", depth)
		fmt.Printf("     %s↳ %s\n", indent, current.Error())
		current = errors.Unwrap(current)
		depth++
	}
}

func RunInterfacesLesson() {
	RunAllInterfaceLessons()
}

func RunAllInterfaceLessons() {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║   📚 LESSON: Interfaces, Generics & Error Trees      ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	ImplicitInterfaces()
	StructEmbedding()
	GenericsDemo()
	ErrorChainsDemo()
}
