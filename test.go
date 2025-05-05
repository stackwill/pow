package main

import (
	"fmt"
	"strings"
)

// TestStruct is a simple struct for demonstration
type TestStruct struct {
	Name   string
	Age    int
	Active bool
}

// Greet returns a greeting message
func (t *TestStruct) Greet() string {
	if !t.Active {
		return "Inactive user"
	}

	return fmt.Sprintf("Hello, %s! You are %d years old.",
		strings.ToUpper(t.Name), t.Age)
}

func main() {
	// Create a new test struct
	person := &TestStruct{
		Name:   "John",
		Age:    30,
		Active: true,
	}

	// Print greeting
	message := person.Greet()
	fmt.Println(message)

	// Some numeric literals
	const (
		Pi     = 3.14159
		Answer = 42
		Binary = 0b1010
		Hex    = 0xFF
	)

	// Try different control structures
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			fmt.Println("Even:", i)
		} else {
			fmt.Println("Odd:", i)
		}
	}

	// String with escape sequences
	fmt.Println("Tabs and newlines: \t\n")

	// Raw string
	multiline := `This is a
multiline string
with "quotes" inside`

	fmt.Println(multiline)
}
