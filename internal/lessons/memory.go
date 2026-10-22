// Package lessons provides executable, heavily-commented Go lessons.
// Each lesson demonstrates core Go mechanics with deliberately buggy/unoptimized
// variations alongside idiomatic fixes, so you learn by seeing the contrast.
package lessons

import "fmt"

// =============================================================================
// LESSON: SLICE HEADER MECHANICS
// =============================================================================
//
// A slice in Go is NOT an array. It's a 3-field struct (a "header"):
//
//   type slice struct {
//       ptr *[...]T  // pointer to an underlying (backing) array
//       len int      // number of elements currently in the slice
//       cap int      // total capacity of the backing array from ptr
//   }
//
// This means slices are VALUE TYPES that contain a POINTER. When you pass a
// slice to a function, you copy the header — but both headers point to the
// SAME backing array. This is the #1 source of subtle bugs for Go beginners.

// SliceMechanics demonstrates the slice header, shared backing arrays, and fixes.
func SliceMechanics() {
	fmt.Println("━━━ Slice Header Mechanics ━━━")

	// --- Part 1: Inspecting the Header ---
	// make([]int, length, capacity) allocates a backing array of `capacity`
	// elements and returns a slice header with len=length, cap=capacity.
	s := make([]int, 3, 5)
	s[0], s[1], s[2] = 10, 20, 30
	fmt.Printf("  s = %v  len=%d  cap=%d\n", s, len(s), cap(s))
	// len(s) = 3 (we can access s[0]..s[2])
	// cap(s) = 5 (the backing array has room for 5 elements total)

	// --- Part 2: THE TRAP — Shared Backing Array ---
	// When you sub-slice, the NEW slice shares the SAME backing array.
	// Appending to the sub-slice writes into the parent's backing array!
	fmt.Println("\n  🐛 BUG: Shared backing array corruption")
	original := []int{1, 2, 3, 4, 5}
	sub := original[1:3] // sub = [2, 3], but cap(sub) = 4 (shares original's array)
	fmt.Printf("  original = %v\n", original)
	fmt.Printf("  sub      = %v  len=%d  cap=%d\n", sub, len(sub), cap(sub))

	// This append does NOT allocate a new array because cap(sub) > len(sub).
	// Instead, it overwrites original[3]!
	sub = append(sub, 999)
	fmt.Printf("  After append(sub, 999):\n")
	fmt.Printf("  original = %v  ← CORRUPTED! original[3] is now 999\n", original)
	fmt.Printf("  sub      = %v\n", sub)

	// --- Part 3: THE FIX — Full Slice Expression ---
	// Use the 3-index slice expression s[low:high:max] to limit capacity.
	// This forces append to allocate a NEW backing array.
	fmt.Println("\n  ✅ FIX 1: Full slice expression s[low:high:max]")
	original2 := []int{1, 2, 3, 4, 5}
	safe := original2[1:3:3] // cap(safe) = 3-1 = 2, same as len
	safe = append(safe, 999) // Must allocate new array — no room!
	fmt.Printf("  original2 = %v  ← SAFE! unchanged\n", original2)
	fmt.Printf("  safe      = %v\n", safe)

	// --- Part 4: THE FIX — Explicit copy() ---
	fmt.Println("\n  ✅ FIX 2: Explicit copy()")
	original3 := []int{1, 2, 3, 4, 5}
	copied := make([]int, 2)
	copy(copied, original3[1:3]) // Copies elements, not the header
	copied = append(copied, 999)
	fmt.Printf("  original3 = %v  ← SAFE! unchanged\n", original3)
	fmt.Printf("  copied    = %v\n", copied)
}

// =============================================================================
// LESSON: VALUE VS. POINTER SEMANTICS
// =============================================================================
//
// Go is a pass-by-value language. EVERYTHING is copied when passed to a function:
// - Structs: the entire struct is copied (shallow copy)
// - Pointers: the pointer value (address) is copied, but it points to the same data
//
// Rule of thumb:
//   - Use value receivers/parameters for small, immutable data
//   - Use pointer receivers/parameters when you need to MODIFY the original
//   - Use pointers for large structs to avoid expensive copies

// User is a simple struct for demonstrating value vs pointer semantics.
type User struct {
	Name string
	Age  int
}

// tryModifyByValue receives a COPY of the struct. Changes are lost.
// WHY: Go copies the entire struct into the function's stack frame.
// The caller's original is untouched.
func tryModifyByValue(u User) {
	u.Age = 100 // Modifies the LOCAL copy only!
}

// modifyByPointer receives a POINTER to the original struct.
// WHY: The pointer is copied, but it still points to the caller's data.
// Dereferencing the pointer modifies the original.
func modifyByPointer(u *User) {
	u.Age = 100 // Modifies the ORIGINAL via pointer dereference
}

// ValueVsPointer demonstrates the critical difference between value and pointer semantics.
func ValueVsPointer() {
	fmt.Println("\n━━━ Value vs. Pointer Semantics ━━━")

	// --- BUGGY: Passing by value ---
	fmt.Println("\n  🐛 BUG: Passing struct by value (change is lost)")
	alice := User{Name: "Alice", Age: 30}
	tryModifyByValue(alice)
	fmt.Printf("  alice.Age = %d  ← Still 30! The function modified a copy.\n", alice.Age)

	// --- FIX: Passing by pointer ---
	fmt.Println("\n  ✅ FIX: Passing struct by pointer (change persists)")
	bob := User{Name: "Bob", Age: 25}
	modifyByPointer(&bob) // Pass address of bob
	fmt.Printf("  bob.Age = %d  ← Changed to 100! Pointer reaches the original.\n", bob.Age)

	// --- Value semantics are SAFE for concurrency ---
	fmt.Println("\n  💡 Insight: Value copies are inherently safe for concurrency.")
	fmt.Println("     Each goroutine gets its own copy — no shared mutable state.")
}

// =============================================================================
// LESSON: ESCAPE ANALYSIS
// =============================================================================
//
// The Go compiler decides whether a variable lives on the STACK or the HEAP.
//   - Stack: fast, automatically freed when function returns
//   - Heap: slower, requires garbage collection
//
// A variable "escapes to the heap" when the compiler can't prove it won't be
// referenced after the function returns (e.g., returning a pointer to a local).
//
// Use `go build -gcflags="-m" ./...` to see escape analysis decisions.

// newUserStack creates a User on the stack (does NOT escape).
// WHY: The returned value is copied to the caller's stack frame.
func newUserStack() User {
	u := User{Name: "Stack", Age: 1}
	return u // Value is COPIED out — u stays on the stack and is freed
}

// newUserHeap creates a User that ESCAPES to the heap.
// WHY: We return a pointer to a local variable. The variable must survive
// after this function returns, so the compiler allocates it on the heap.
func newUserHeap() *User {
	u := User{Name: "Heap", Age: 2}
	return &u // u ESCAPES — compiler moves it to the heap
}

// EscapeAnalysisDemo shows the difference between stack and heap allocation.
func EscapeAnalysisDemo() {
	fmt.Println("\n━━━ Escape Analysis ━━━")

	stackUser := newUserStack()
	heapUser := newUserHeap()

	fmt.Printf("  Stack user: %+v (returned by value, allocated on stack)\n", stackUser)
	fmt.Printf("  Heap user:  %+v (returned by pointer, escaped to heap)\n", *heapUser)

	fmt.Println("\n  🔬 To see escape decisions, run:")
	fmt.Println("     go build -gcflags=\"-m\" ./internal/lessons/")
	fmt.Println("  Look for 'moved to heap' and 'does not escape' annotations.")

	// --- Why this matters ---
	fmt.Println("\n  💡 Performance insight:")
	fmt.Println("     Heap allocations add GC pressure. In hot paths, prefer")
	fmt.Println("     returning values (not pointers) for small structs.")
	fmt.Println("     The compiler is smart — it only escapes when necessary.")
}

// RunMemoryLesson is an alias for RunAllMemoryLessons to match the entrypoint runner.
func RunMemoryLesson() {
	RunAllMemoryLessons()
}

// RunAllMemoryLessons executes all memory-related lessons in sequence.
func RunAllMemoryLessons() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║     📚 LESSON: Go Memory Mechanics           ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	SliceMechanics()
	ValueVsPointer()
	EscapeAnalysisDemo()
}
