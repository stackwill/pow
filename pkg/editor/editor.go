package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"pow/pkg/config"
)

// Editor represents the text editor application
type Editor struct {
	screen   tcell.Screen
	filePath string
	content  []string
	theme    *config.Theme

	// Editing state
	cursorX  int
	cursorY  int
	modified bool
	quit     chan struct{}
}

// NewEditor creates a new editor instance
func NewEditor(filePath string) (*Editor, error) {
	var content []string
	fileExists := true

	// Try to load the file if it exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fileExists = false
		content = []string{""}
		fmt.Fprintln(os.Stderr, "New file")
	} else {
		var err error
		content, err = loadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	// Ensure we always have at least one line
	if len(content) == 0 {
		content = []string{""}
	}

	// Load theme configuration
	theme, themeErr := config.LoadTheme(filepath.Join("config", "theme.conf"))
	if themeErr != nil {
		// Just print the error, don't abort - we'll use the default theme
		fmt.Fprintln(os.Stderr, themeErr)
	}

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := screen.Init(); err != nil {
		return nil, err
	}

	// Create editor instance
	editor := &Editor{
		screen:   screen,
		filePath: filePath,
		content:  content,
		theme:    theme,
		cursorX:  0,
		cursorY:  0,
		modified: !fileExists, // Mark as modified if it's a new file
		quit:     make(chan struct{}),
	}

	return editor, nil
}

// Run starts the editor application
func (e *Editor) Run() error {
	// Set default background color for entire screen
	e.screen.SetStyle(tcell.StyleDefault.
		Foreground(e.theme.TextColor).
		Background(e.theme.BackgroundColor))

	// Draw the initial screen content
	e.draw()

	// Main event loop
	for {
		ev := e.screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			e.screen.Sync()
			e.draw()

		case *tcell.EventKey:
			if !e.handleKeyEvent(ev) {
				return nil // Exit requested
			}
			e.draw()

		}
	}
}

// draw renders the editor content to the screen
func (e *Editor) draw() {
	e.screen.Clear()

	// Get screen dimensions
	width, height := e.screen.Size()

	// Set default style for background
	defaultStyle := tcell.StyleDefault.
		Foreground(e.theme.TextColor).
		Background(e.theme.BackgroundColor)

	// Fill entire screen with background color
	for y := 0; y < height-1; y++ { // Leave the last line for status
		for x := 0; x < width; x++ {
			e.screen.SetContent(x, y, ' ', nil, defaultStyle)
		}
	}

	// Render content
	for y, line := range e.content {
		if y >= height-1 { // Leave the last line for status
			break
		}

		// Draw the line
		for x, r := range line {
			if x >= width {
				break
			}

			// Skip cursor position, we'll draw it separately
			if y == e.cursorY && x == e.cursorX {
				continue
			}

			e.screen.SetContent(x, y, r, nil, defaultStyle)
		}
	}

	// Draw cursor
	if e.cursorY < height-1 {
		// Get char under cursor
		var cursorChar rune = ' ' // Default to space
		if e.cursorY < len(e.content) {
			line := e.content[e.cursorY]
			if e.cursorX < len(line) {
				cursorChar = rune(line[e.cursorX])
			}
		}

		// Set cursor style with themed cursor color
		cursorStyle := tcell.StyleDefault.
			Foreground(e.theme.BackgroundColor).
			Background(e.theme.CursorColor)

		// Draw the cursor
		e.screen.SetContent(e.cursorX, e.cursorY, cursorChar, nil, cursorStyle)
	}

	// Draw status line
	statusStyle := tcell.StyleDefault.
		Foreground(e.theme.StatusForeground).
		Background(e.theme.StatusBackground)

	// Fill status line with background color
	for x := 0; x < width; x++ {
		e.screen.SetContent(x, height-1, ' ', nil, statusStyle)
	}

	// Add text to status line
	modifiedIndicator := " "
	if e.modified {
		modifiedIndicator = "*"
	}

	statusText := fmt.Sprintf(" %s%s [%d:%d]", modifiedIndicator, e.filePath, e.cursorY+1, e.cursorX+1)

	for x, r := range statusText {
		if x < width {
			e.screen.SetContent(x, height-1, r, nil, statusStyle)
		}
	}

	// Show the result
	e.screen.Show()
}

// handleKeyEvent processes keyboard input events
func (e *Editor) handleKeyEvent(ev *tcell.EventKey) bool {
	// Handle key events
	switch ev.Key() {
	case tcell.KeyCtrlC: // Legacy exit - immediately quit
		close(e.quit)
		e.screen.Fini()
		return false

	case tcell.KeyCtrlX: // Exit with prompt if modified
		if e.modified {
			return e.promptSaveBeforeExit()
		}
		close(e.quit)
		e.screen.Fini()
		return false

	case tcell.KeyCtrlS: // Save file
		e.saveFile()
		return true

	case tcell.KeyUp:
		if e.cursorY > 0 {
			e.cursorY--
			// Make sure cursorX is not beyond end of line
			if e.cursorX > len(e.content[e.cursorY]) {
				e.cursorX = len(e.content[e.cursorY])
			}
		}
		return true

	case tcell.KeyDown:
		if e.cursorY < len(e.content)-1 {
			e.cursorY++
			// Make sure cursorX is not beyond end of line
			if e.cursorX > len(e.content[e.cursorY]) {
				e.cursorX = len(e.content[e.cursorY])
			}
		}
		return true

	case tcell.KeyLeft:
		if e.cursorX > 0 {
			e.cursorX--
		} else if e.cursorY > 0 {
			// Move to end of previous line
			e.cursorY--
			e.cursorX = len(e.content[e.cursorY])
		}
		return true

	case tcell.KeyRight:
		if e.cursorY < len(e.content) && e.cursorX < len(e.content[e.cursorY]) {
			e.cursorX++
		} else if e.cursorY < len(e.content)-1 {
			// Move to beginning of next line
			e.cursorY++
			e.cursorX = 0
		}
		return true

	case tcell.KeyEnter:
		// Split the current line at cursor position
		currentLine := e.content[e.cursorY]
		leftPart := currentLine[:e.cursorX]
		rightPart := ""
		if e.cursorX < len(currentLine) {
			rightPart = currentLine[e.cursorX:]
		}

		// Update current line to be everything before cursor
		e.content[e.cursorY] = leftPart

		// Insert new line with everything after cursor
		newContent := make([]string, len(e.content)+1)
		copy(newContent, e.content[:e.cursorY+1])
		newContent[e.cursorY+1] = rightPart
		copy(newContent[e.cursorY+2:], e.content[e.cursorY+1:])
		e.content = newContent

		// Move cursor to beginning of new line
		e.cursorY++
		e.cursorX = 0
		e.modified = true
		return true

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if e.cursorX > 0 {
			// Remove character before cursor
			currentLine := e.content[e.cursorY]
			e.content[e.cursorY] = currentLine[:e.cursorX-1] + currentLine[e.cursorX:]
			e.cursorX--
			e.modified = true
		} else if e.cursorY > 0 {
			// At beginning of line, join with previous line
			currentLine := e.content[e.cursorY]
			prevLine := e.content[e.cursorY-1]

			// Set cursor position to end of previous line
			e.cursorX = len(prevLine)

			// Join lines
			e.content[e.cursorY-1] = prevLine + currentLine

			// Remove current line
			newContent := make([]string, len(e.content)-1)
			copy(newContent, e.content[:e.cursorY])
			copy(newContent[e.cursorY:], e.content[e.cursorY+1:])
			e.content = newContent

			e.cursorY--
			e.modified = true
		}
		return true

	case tcell.KeyDelete:
		if e.cursorY < len(e.content) {
			currentLine := e.content[e.cursorY]
			if e.cursorX < len(currentLine) {
				// Remove character at cursor
				e.content[e.cursorY] = currentLine[:e.cursorX] + currentLine[e.cursorX+1:]
				e.modified = true
			} else if e.cursorY < len(e.content)-1 {
				// At end of line, join with next line
				nextLine := e.content[e.cursorY+1]

				// Join lines
				e.content[e.cursorY] = currentLine + nextLine

				// Remove next line
				newContent := make([]string, len(e.content)-1)
				copy(newContent, e.content[:e.cursorY+1])
				copy(newContent[e.cursorY+1:], e.content[e.cursorY+2:])
				e.content = newContent

				e.modified = true
			}
		}
		return true
	}

	// Handle regular character input
	if ev.Key() == tcell.KeyRune {
		r := ev.Rune()

		// Insert the character at cursor position
		currentLine := e.content[e.cursorY]
		newLine := ""
		if e.cursorX > len(currentLine) {
			// If cursor is beyond the end of the line, pad with spaces
			newLine = currentLine + strings.Repeat(" ", e.cursorX-len(currentLine)) + string(r)
		} else {
			newLine = currentLine[:e.cursorX] + string(r) + currentLine[e.cursorX:]
		}

		e.content[e.cursorY] = newLine
		e.cursorX++
		e.modified = true
		return true
	}

	// Pass other keys through
	return true
}

// saveFile saves the current content to the file
func (e *Editor) saveFile() {
	// If it's a new file and no path is set, prompt for a filename
	if e.filePath == "" {
		e.promptForFilename()
		return
	}

	content := strings.Join(e.content, "\n")

	err := os.WriteFile(e.filePath, []byte(content), 0644)
	if err != nil {
		e.showMessage(fmt.Sprintf("Error saving file: %v", err))
		return
	}

	e.modified = false
}

// promptForFilename asks the user for a filename to save
func (e *Editor) promptForFilename() {
	// Save current content to restore it later
	width, height := e.screen.Size()

	// Create an input field at the bottom of the screen
	prompt := "Enter filename: "
	input := ""

	// Process input until Enter or Esc is pressed
	for {
		// Clear the status line
		for x := 0; x < width; x++ {
			e.screen.SetContent(x, height-1, ' ', nil, tcell.StyleDefault.
				Foreground(tcell.ColorWhite).
				Background(tcell.ColorBlue))
		}

		// Display prompt and input
		for i, c := range prompt {
			e.screen.SetContent(i, height-1, c, nil, tcell.StyleDefault.
				Foreground(tcell.ColorWhite).
				Background(tcell.ColorBlue))
		}

		for i, c := range input {
			e.screen.SetContent(len(prompt)+i, height-1, c, nil, tcell.StyleDefault.
				Foreground(tcell.ColorWhite).
				Background(tcell.ColorBlue))
		}

		// Show cursor
		e.screen.SetContent(len(prompt)+len(input), height-1, ' ', nil, tcell.StyleDefault.
			Foreground(tcell.ColorBlue).
			Background(tcell.ColorWhite))

		e.screen.Show()

		// Wait for key event
		ev := e.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEnter:
				if input != "" {
					e.filePath = input
					e.saveFile()
					return
				}
			case tcell.KeyEscape:
				return
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			case tcell.KeyRune:
				input += string(ev.Rune())
			}
		}
	}
}

// promptSaveBeforeExit asks the user if they want to save before exiting
func (e *Editor) promptSaveBeforeExit() bool {
	width, height := e.screen.Size()

	// Options
	options := []string{"Yes", "No", "Cancel"}
	selected := 0

	message := "Save changes before exiting?"

	for {
		// Clear the dialog area
		dialogWidth := 40
		dialogHeight := 5
		dialogX := (width - dialogWidth) / 2
		dialogY := (height - dialogHeight) / 2

		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				if x >= 0 && x < width && y >= 0 && y < height {
					style := tcell.StyleDefault.
						Foreground(tcell.ColorBlack).
						Background(tcell.ColorWhite)

					// Border
					if x == dialogX || x == dialogX+dialogWidth-1 || y == dialogY || y == dialogY+dialogHeight-1 {
						e.screen.SetContent(x, y, ' ', nil, style)
					} else {
						e.screen.SetContent(x, y, ' ', nil, style)
					}
				}
			}
		}

		// Draw message
		for i, c := range message {
			msgX := dialogX + (dialogWidth-len(message))/2 + i
			if msgX >= 0 && msgX < width {
				e.screen.SetContent(msgX, dialogY+1, c, nil, tcell.StyleDefault.
					Foreground(tcell.ColorBlack).
					Background(tcell.ColorWhite))
			}
		}

		// Draw options
		totalOptionsWidth := 0
		for _, opt := range options {
			totalOptionsWidth += len(opt) + 2 // +2 for spacing
		}

		optionX := dialogX + (dialogWidth-totalOptionsWidth)/2

		for i, opt := range options {
			style := tcell.StyleDefault.
				Foreground(tcell.ColorBlack).
				Background(tcell.ColorWhite)

			if i == selected {
				style = tcell.StyleDefault.
					Foreground(tcell.ColorWhite).
					Background(tcell.ColorBlue)
			}

			for j, c := range opt {
				if optionX+j < width {
					e.screen.SetContent(optionX+j, dialogY+3, c, nil, style)
				}
			}

			optionX += len(opt) + 2
		}

		e.screen.Show()

		// Wait for key event
		ev := e.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyLeft:
				selected = (selected + len(options) - 1) % len(options)
			case tcell.KeyRight:
				selected = (selected + 1) % len(options)
			case tcell.KeyEnter:
				switch options[selected] {
				case "Yes":
					e.saveFile()
					if !e.modified { // Only exit if save was successful
						close(e.quit)
						e.screen.Fini()
						return false
					}
					return true
				case "No":
					close(e.quit)
					e.screen.Fini()
					return false
				case "Cancel":
					return true
				}
			case tcell.KeyEscape:
				return true
			}
		}
	}
}

// showMessage displays a message to the user
func (e *Editor) showMessage(message string) {
	width, height := e.screen.Size()

	// Display message in the middle of the screen
	dialogWidth := len(message) + 4
	dialogHeight := 5
	if dialogWidth < 20 {
		dialogWidth = 20
	}

	dialogX := (width - dialogWidth) / 2
	dialogY := (height - dialogHeight) / 2

	// Draw dialog box
	for y := dialogY; y < dialogY+dialogHeight; y++ {
		for x := dialogX; x < dialogX+dialogWidth; x++ {
			if x >= 0 && x < width && y >= 0 && y < height {
				style := tcell.StyleDefault.
					Foreground(tcell.ColorBlack).
					Background(tcell.ColorWhite)

				e.screen.SetContent(x, y, ' ', nil, style)
			}
		}
	}

	// Draw message
	for i, c := range message {
		msgX := dialogX + (dialogWidth-len(message))/2 + i
		if msgX >= 0 && msgX < width {
			e.screen.SetContent(msgX, dialogY+1, c, nil, tcell.StyleDefault.
				Foreground(tcell.ColorBlack).
				Background(tcell.ColorWhite))
		}
	}

	// Draw OK button
	okText := "OK"
	okX := dialogX + (dialogWidth-len(okText))/2

	style := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlue)

	for i, c := range okText {
		if okX+i < width {
			e.screen.SetContent(okX+i, dialogY+3, c, nil, style)
		}
	}

	e.screen.Show()

	// Wait for any key press
	for {
		ev := e.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyEnter || ev.Key() == tcell.KeyEscape {
				return
			}
		}
	}
}

// loadFile reads the content of a file into memory
func loadFile(filePath string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Split content into lines
	lines := strings.Split(string(content), "\n")

	return lines, nil
}
