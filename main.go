package main

import (
	"fmt"
	"os"
	"pow/pkg/editor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: pow <filename>")
		os.Exit(1)
	}

	filename := os.Args[1]

	app, err := editor.NewEditor(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing editor: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running editor: %v\n", err)
		os.Exit(1)
	}
}
