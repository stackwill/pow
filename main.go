package main

import (
	"fmt"
	"os"
	"pow/pkg/editor"
)

func main() {
	var filename string
	var err error
	var app *editor.Editor

	// Check if a filename was provided as an argument
	if len(os.Args) > 1 {
		filename = os.Args[1]
		app, err = editor.NewEditor(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing editor: %v\n", err)
			os.Exit(1)
		}
	} else {
		// No filename provided, initialize with empty file
		app, err = editor.NewEditor("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing editor: %v\n", err)
			os.Exit(1)
		}
	}

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running editor: %v\n", err)
		os.Exit(1)
	}
}
