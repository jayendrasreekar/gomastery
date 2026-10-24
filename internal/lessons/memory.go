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
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 1. SLICE HEADER MECHANICS & CAPACITY")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// --- Part 1: Inspecting the Header ---
	fmt.Println("\n▶ Part 1: Creating a slice with length and capacity")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ s := make([]int, 3, 5)                            │")
	fmt.Println("  │ s[0], s[1], s[2] = 10, 20, 30                     │")
	fmt.Printf("%s\n", "  │ fmt.Printf(\"s = %v  len=%d  cap=%d\\n\", s, ...)    │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	s := make([]int, 3, 5)
	s[0], s[1], s[2] = 10, 20, 30
	fmt.Printf("  👉 Output:  s = %v  len=%d  cap=%d\n", s, len(s), cap(s))
	fmt.Println("  💡 Insight: len=3 (accessible elements: s[0]..s[2])")
	fmt.Println("             cap=5 (backing array has room for 5 total elements)")

	// --- Part 2: THE TRAP — Shared Backing Array ---
	fmt.Println("\n▶ Part 2: THE TRAP — Sub-slicing shares the backing array")
	fmt.Println("  ┌──[ The Buggy Code ]───────────────────────────────┐")
	fmt.Println("  │ original := []int{1, 2, 3, 4, 5}                  │")
	fmt.Println("  │ sub := original[1:3]     // cap(sub) = 4 !        │")
	fmt.Println("  │ sub = append(sub, 999)   // Overwrites original!  │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	original := []int{1, 2, 3, 4, 5}
	sub := original[1:3]
	sub = append(sub, 999)
	fmt.Printf("  🐛 Result:  original = %v  ← CORRUPTED! original[3] is now 999\n", original)
	fmt.Printf("             sub      = %v\n", sub)
	fmt.Println("  ⚠️ Why:     Because cap(sub) was 4, append did NOT reallocate;")
	fmt.Println("             it silently wrote into original's 4th slot!")

	// --- Part 3: THE FIX — Full Slice Expression ---
	fmt.Println("\n▶ Part 3: THE FIX — Full Slice Expression s[low:high:max]")
	fmt.Println("  ┌──[ The Safe Code ]────────────────────────────────┐")
	fmt.Println("  │ original2 := []int{1, 2, 3, 4, 5}                 │")
	fmt.Println("  │ safe := original2[1:3:3]  // cap = 3 - 1 = 2      │")
	fmt.Println("  │ safe = append(safe, 999)  // Forces new array!    │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	original2 := []int{1, 2, 3, 4, 5}
	safe := original2[1:3:3]
	safe = append(safe, 999)
	fmt.Printf("  ✅ Result:  original2 = %v  ← SAFE! original unchanged\n", original2)
	fmt.Printf("             safe      = %v\n", safe)
	fmt.Println("  💡 Why:     Setting max=3 forced cap=2 (equal to len).")
	fmt.Println("             append() had no room, so it allocated a new array!")

	// --- Part 4: THE FIX — Explicit copy() ---
	fmt.Println("\n▶ Part 4: THE FIX — Explicit copy() to independent memory")
	fmt.Println("  ┌──[ The Copy Code ]────────────────────────────────┐")
	fmt.Println("  │ original3 := []int{1, 2, 3, 4, 5}                 │")
	fmt.Println("  │ copied := make([]int, 2)                          │")
	fmt.Println("  │ copy(copied, original3[1:3])                      │")
	fmt.Println("  │ copied = append(copied, 999)                      │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	original3 := []int{1, 2, 3, 4, 5}
	copied := make([]int, 2)
	copy(copied, original3[1:3])
	copied = append(copied, 999)
	fmt.Printf("  ✅ Result:  original3 = %v  ← SAFE! original unchanged\n", original3)
	fmt.Printf("             copied    = %v\n", copied)
}

// =============================================================================
// LESSON: VALUE VS. POINTER SEMANTICS
// =============================================================================

// User is a simple struct for demonstrating value vs pointer semantics.
type User struct {
	Name string
	Age  int
}

// tryModifyByValue receives a COPY of the struct. Changes are lost.
func tryModifyByValue(u User) {
	u.Age = 100 // Modifies the LOCAL copy only!
}

// modifyByPointer receives a POINTER to the original struct.
func modifyByPointer(u *User) {
	u.Age = 100 // Modifies the ORIGINAL via pointer dereference
}

// ValueVsPointer demonstrates the critical difference between value and pointer semantics.
func ValueVsPointer() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 2. VALUE VS. POINTER SEMANTICS")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// --- BUGGY: Passing by value ---
	fmt.Println("\n▶ Value Semantics: Structs are copied on function call")
	fmt.Println("  ┌──[ The Buggy Code ]───────────────────────────────┐")
	fmt.Println("  │ func tryModifyByValue(u User) { u.Age = 100 }     │")
	fmt.Println("  │ alice := User{Name: \"Alice\", Age: 30}             │")
	fmt.Println("  │ tryModifyByValue(alice)                           │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	alice := User{Name: "Alice", Age: 30}
	tryModifyByValue(alice)
	fmt.Printf("  🐛 Result:  alice.Age = %d  ← Still 30! Function modified a copy.\n", alice.Age)

	// --- FIX: Passing by pointer ---
	fmt.Println("\n▶ Pointer Semantics: Pointer reaches caller's original memory")
	fmt.Println("  ┌──[ The Correct Code ]─────────────────────────────┐")
	fmt.Println("  │ func modifyByPointer(u *User) { u.Age = 100 }     │")
	fmt.Println("  │ bob := User{Name: \"Bob\", Age: 25}                 │")
	fmt.Println("  │ modifyByPointer(&bob)                             │")
	fmt.Println("  └───────────────────────────────────────────────────┘")
	bob := User{Name: "Bob", Age: 25}
	modifyByPointer(&bob)
	fmt.Printf("  ✅ Result:  bob.Age = %d  ← Changed to 100! Pointer reached original.\n", bob.Age)
	fmt.Println("  💡 Rule:    Use pointers when mutating state or avoiding large copies.")
	fmt.Println("             Use values for immutable, concurrency-safe data.")
}

// =============================================================================
// LESSON: ESCAPE ANALYSIS
// =============================================================================

func newUserStack() User {
	u := User{Name: "Stack", Age: 1}
	return u // Value is COPIED out — u stays on the stack and is freed
}

func newUserHeap() *User {
	u := User{Name: "Heap", Age: 2}
	return &u // u ESCAPES — compiler moves it to the heap
}

// EscapeAnalysisDemo shows the difference between stack and heap allocation.
func EscapeAnalysisDemo() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 3. ESCAPE ANALYSIS (STACK VS. HEAP)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ┌──[ Code ]─────────────────────────────────────────┐")
	fmt.Println("  │ func newUserStack() User {                        │")
	fmt.Println("  │     return User{Name: \"Stack\", Age: 1} // Stack    │")
	fmt.Println("  │ }                                                 │")
	fmt.Println("  │ func newUserHeap() *User {                        │")
	fmt.Println("  │     u := User{Name: \"Heap\", Age: 2}               │")
	fmt.Println("  │     return &u                          // Heap!   │")
	fmt.Println("  │ }                                                 │")
	fmt.Println("  └───────────────────────────────────────────────────┘")

	stackUser := newUserStack()
	heapUser := newUserHeap()

	fmt.Printf("  👉 Output:  Stack user = %+v (freed automatically when frame pops)\n", stackUser)
	fmt.Printf("             Heap user  = %+v (survives via GC pointer)\n", *heapUser)
	fmt.Println("\n  🔬 Terminal inspection command:")
	fmt.Println("     go build -gcflags=\"-m\" ./internal/lessons/")
}

// RunMemoryLesson is an alias for RunAllMemoryLessons to match the entrypoint runner.
func RunMemoryLesson() {
	RunAllMemoryLessons()
}

// RunAllMemoryLessons executes all memory-related lessons in sequence.
func RunAllMemoryLessons() {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║     📚 LESSON: Go Memory Mechanics & Slices          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	SliceMechanics()
	ValueVsPointer()
	EscapeAnalysisDemo()
}
