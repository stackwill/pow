package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"pow/pkg/config"
	"pow/pkg/syntax"
)

// Editor represents the text editor application
type Editor struct {
	screen      tcell.Screen
	filePath    string
	content     []string
	theme       *config.Theme
	highlighter *syntax.Highlighter

	// Editing state
	cursorX  int
	cursorY  int
	scrollY  int // Track vertical scroll position
	modified bool
	quit     chan struct{}
}

// NewEditor creates a new editor instance
func NewEditor(filePath string) (*Editor, error) {
	var content []string
	fileExists := true

	// Check if a file path was provided
	if filePath == "" {
		content = []string{""}
		fileExists = false
		filePath = "untitled.txt" // Use a default filename but don't save yet
	} else {
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
	}

	// Ensure we always have at least one line
	if len(content) == 0 {
		content = []string{""}
	}

	// Load theme configuration
	theme, themeErr := config.LoadTheme(filepath.Join("config", "config.conf"))
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

	// Initialize syntax highlighter
	highlighter := syntax.NewHighlighter(filePath)

	// Create editor instance
	editor := &Editor{
		screen:      screen,
		filePath:    filePath,
		content:     content,
		theme:       theme,
		highlighter: highlighter,
		cursorX:     0,
		cursorY:     0,
		scrollY:     0,
		modified:    !fileExists, // Mark as modified if it's a new file
		quit:        make(chan struct{}),
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

	// Ensure cursor is visible
	e.ensureVisibleCursor()

	// Get the entire file content as a single string for syntax highlighting
	content := strings.Join(e.content, "\n")

	// Highlight the entire content
	highlightedLines := e.highlighter.HighlightContent(content)

	// Calculate the visible range of lines
	visibleStart := e.scrollY
	visibleEnd := e.scrollY + (height - 1) // Leave space for status line

	// Allow displaying one line beyond content
	maxVisibleEnd := len(e.content) + 1
	if visibleEnd > maxVisibleEnd {
		visibleEnd = maxVisibleEnd
	}

	// Render visible content
	for i := visibleStart; i < visibleEnd; i++ {
		// Calculate screen position
		y := i - e.scrollY

		// Only render content if within actual content range
		if i < len(e.content) {
			line := e.content[i]

			// Get the highlighted segments for this line
			var colorSegments []syntax.ColorSegment
			if i < len(highlightedLines) {
				colorSegments = highlightedLines[i].Colors
			}

			// Draw the line with syntax highlighting
			for x, r := range line {
				if x >= width {
					break
				}

				// Skip cursor position, we'll draw it separately
				if i == e.cursorY && x == e.cursorX {
					continue
				}

				// Default to using the default style
				style := defaultStyle

				// Check if we have a highlighted segment that includes this position
				for _, segment := range colorSegments {
					if x >= segment.StartCol && x < segment.EndCol {
						// Apply the highlight style but preserve background color
						style = segment.Style.Background(e.theme.BackgroundColor)
						break
					}
				}

				e.screen.SetContent(x, y, r, nil, style)
			}
		}
		// The extra line beyond content is already drawn as empty space
	}

	// Draw cursor (only if it's in the visible area)
	if e.cursorY >= e.scrollY && e.cursorY < e.scrollY+height-1 {
		// Get cursor screen position
		cursorScreenY := e.cursorY - e.scrollY

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
		e.screen.SetContent(e.cursorX, cursorScreenY, cursorChar, nil, cursorStyle)
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

	// Get file type from highlighter
	fileType := e.highlighter.GetFileType()

	// Show scroll position information
	scrollInfo := ""
	if len(e.content) > height-1 {
		totalLines := len(e.content)
		visibleStart := e.scrollY + 1
		visibleEnd := min(e.scrollY+height-1, totalLines)
		scrollPercentage := 100 * visibleEnd / totalLines
		scrollInfo = fmt.Sprintf(" [%d-%d/%d %d%%]", visibleStart, visibleEnd, totalLines, scrollPercentage)
	}

	statusText := fmt.Sprintf(" %s%s [%s] [%d:%d]%s",
		modifiedIndicator, e.filePath, fileType, e.cursorY+1, e.cursorX+1, scrollInfo)

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
	// Get screen dimensions
	_, height := e.screen.Size()
	contentHeight := height - 1 // Subtract status line

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
		// Allow moving to one line beyond content
		maxY := len(e.content)
		if e.cursorY < maxY {
			e.cursorY++
			// If we're on a regular line, adjust cursor X if needed
			if e.cursorY < maxY && e.cursorX > len(e.content[e.cursorY]) {
				e.cursorX = len(e.content[e.cursorY])
			} else if e.cursorY == maxY {
				// We're on the extra line beyond content
				e.cursorX = 0
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
		// If we're on a normal line and can move right
		if e.cursorY < len(e.content) && e.cursorX < len(e.content[e.cursorY]) {
			e.cursorX++
		} else if e.cursorY < len(e.content) {
			// At the end of a normal line, move to the next line
			e.cursorY++
			e.cursorX = 0
		}
		return true

	case tcell.KeyPgUp:
		// Move cursor up by a page
		if e.cursorY > 0 {
			e.cursorY -= contentHeight
			if e.cursorY < 0 {
				e.cursorY = 0
			}
			// Make sure cursorX is valid for the new line
			if e.cursorX > len(e.content[e.cursorY]) {
				e.cursorX = len(e.content[e.cursorY])
			}
		}
		return true

	case tcell.KeyPgDn:
		// Move cursor down by a page
		if e.cursorY < len(e.content)-1 {
			e.cursorY += contentHeight
			if e.cursorY >= len(e.content) {
				e.cursorY = len(e.content) - 1
			}
			// Make sure cursorX is valid for the new line
			if e.cursorX > len(e.content[e.cursorY]) {
				e.cursorX = len(e.content[e.cursorY])
			}
		}
		return true

	case tcell.KeyHome:
		// Move to beginning of line
		e.cursorX = 0
		return true

	case tcell.KeyEnd:
		// Move to end of line
		if e.cursorY < len(e.content) {
			e.cursorX = len(e.content[e.cursorY])
		}
		return true

	case tcell.KeyEnter:
		// Handle enter at the extra line - append a new line
		if e.cursorY == len(e.content) {
			// Add a new empty line
			e.content = append(e.content, "")
			e.cursorY = len(e.content) - 1
			e.cursorX = 0
			e.modified = true
			return true
		}

		// Normal case - split the current line at cursor position
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
			// Delete the character before the cursor
			currentLine := e.content[e.cursorY]
			e.content[e.cursorY] = currentLine[:e.cursorX-1] + currentLine[e.cursorX:]
			e.cursorX--
			e.modified = true
		} else if e.cursorY > 0 {
			// We're at the beginning of a line, merge with the previous line
			previousLine := e.content[e.cursorY-1]
			currentLine := e.content[e.cursorY]

			// Set cursor to the end of the previous line
			e.cursorX = len(previousLine)

			// Merge the lines
			e.content[e.cursorY-1] = previousLine + currentLine

			// Remove the current line
			newContent := make([]string, len(e.content)-1)
			copy(newContent, e.content[:e.cursorY])
			copy(newContent[e.cursorY:], e.content[e.cursorY+1:])
			e.content = newContent

			// Move cursor up to the previous line
			e.cursorY--
			e.modified = true
		}
		return true

	case tcell.KeyDelete:
		if e.cursorY < len(e.content) {
			currentLine := e.content[e.cursorY]
			if e.cursorX < len(currentLine) {
				// Delete character at cursor
				e.content[e.cursorY] = currentLine[:e.cursorX] + currentLine[e.cursorX+1:]
				e.modified = true
			} else if e.cursorY < len(e.content)-1 {
				// At the end of the line, merge with next line
				nextLine := e.content[e.cursorY+1]
				e.content[e.cursorY] = currentLine + nextLine

				// Remove the next line
				newContent := make([]string, len(e.content)-1)
				copy(newContent, e.content[:e.cursorY+1])
				copy(newContent[e.cursorY+1:], e.content[e.cursorY+2:])
				e.content = newContent
				e.modified = true
			}
		}
		return true

	case tcell.KeyTab:
		// Insert a tab (4 spaces for now)
		currentLine := e.content[e.cursorY]
		if e.cursorX > len(currentLine) {
			e.content[e.cursorY] = currentLine + strings.Repeat(" ", e.cursorX-len(currentLine)) + "    "
		} else {
			e.content[e.cursorY] = currentLine[:e.cursorX] + "    " + currentLine[e.cursorX:]
		}
		e.cursorX += 4
		e.modified = true
		return true

	case tcell.KeyRune:
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
	// If no path is set, prompt for a filename
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

	// Update highlighter in case file type changed
	e.highlighter = syntax.NewHighlighter(e.filePath)
}

// promptForFilename asks the user for a filename to save
func (e *Editor) promptForFilename() {
	width, height := e.screen.Size()

	// Dialog dimensions
	dialogWidth := min(60, width-4)
	dialogHeight := 5
	dialogX := (width - dialogWidth) / 2
	dialogY := (height - dialogHeight) / 2

	// Create styles
	dialogStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	borderStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogBorderColor).
		Background(e.theme.DialogBackground)

	titleStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogSelectedForeground).
		Background(e.theme.DialogButtonBackground)

	inputStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	cursorStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogBackground).
		Background(e.theme.DialogSelectedBackground)

	// Create an input field at the bottom of the screen
	prompt := "Enter filename: "
	input := ""
	title := " Save File "

	// Process input until Enter or Esc is pressed
	for {
		// Draw dialog background
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Simple rectangle with solid background
				if y == dialogY || y == dialogY+dialogHeight-1 ||
					x == dialogX || x == dialogX+dialogWidth-1 {
					e.screen.SetContent(x, y, ' ', nil, borderStyle)
				} else {
					e.screen.SetContent(x, y, ' ', nil, dialogStyle)
				}
			}
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Draw prompt
		promptX := dialogX + 2
		for i, c := range prompt {
			e.screen.SetContent(promptX+i, dialogY+2, c, nil, inputStyle)
		}

		// Draw input
		inputX := promptX + len(prompt)
		for i, c := range input {
			if inputX+i < dialogX+dialogWidth-2 {
				e.screen.SetContent(inputX+i, dialogY+2, c, nil, inputStyle)
			}
		}

		// Show cursor
		e.screen.SetContent(inputX+len(input), dialogY+2, ' ', nil, cursorStyle)

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
					// Update highlighter in case file type changed
					e.highlighter = syntax.NewHighlighter(e.filePath)
					return
				}
			case tcell.KeyEscape:
				// If no filename is set and user cancels, use a default name
				if e.filePath == "" {
					e.filePath = "untitled.txt"
				}
				return
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			case tcell.KeyRune:
				// Only add the character if it would fit in the dialog
				if inputX+len(input) < dialogX+dialogWidth-3 {
					input += string(ev.Rune())
				}
			}
		}
	}
}

// promptSaveBeforeExit asks the user if they want to save before exiting
func (e *Editor) promptSaveBeforeExit() bool {
	width, height := e.screen.Size()

	// Options
	options := []string{"Save", "Don't Save", "Cancel"}
	selected := 0

	message := "Save changes before exiting?"

	// Dialog dimensions
	dialogWidth := min(50, width-4)
	dialogHeight := 7
	dialogX := (width - dialogWidth) / 2
	dialogY := (height - dialogHeight) / 2

	// Create styles
	dialogStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	borderStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogBorderColor).
		Background(e.theme.DialogBackground)

	titleStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogSelectedForeground).
		Background(e.theme.DialogButtonBackground)

	textStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	buttonStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogButtonForeground).
		Background(e.theme.DialogButtonBackground)

	selectedStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogSelectedForeground).
		Background(e.theme.DialogSelectedBackground)

	title := " Confirm Exit "

	for {
		// Clear the dialog area
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Simple rectangle with solid background
				if y == dialogY || y == dialogY+dialogHeight-1 ||
					x == dialogX || x == dialogX+dialogWidth-1 {
					e.screen.SetContent(x, y, ' ', nil, borderStyle)
				} else {
					e.screen.SetContent(x, y, ' ', nil, dialogStyle)
				}
			}
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Draw message
		for i, r := range message {
			x := dialogX + (dialogWidth-len(message))/2 + i
			y := dialogY + 2
			if x >= dialogX && x < dialogX+dialogWidth-1 {
				e.screen.SetContent(x, y, r, nil, textStyle)
			}
		}

		// Draw buttons
		buttonY := dialogY + 4

		// Calculate total width of all buttons with spacing
		totalButtonWidth := 0
		for _, opt := range options {
			totalButtonWidth += len(opt) + 4 // Add padding around button text
		}
		totalButtonWidth += (len(options) - 1) * 2 // Add spacing between buttons

		// Start position for first button
		buttonX := dialogX + (dialogWidth-totalButtonWidth)/2

		for i, opt := range options {
			// Draw button with simple background
			buttonWidth := len(opt) + 4

			// Button background
			for y := buttonY; y <= buttonY+2; y++ {
				for x := buttonX; x < buttonX+buttonWidth; x++ {
					style := buttonStyle
					if i == selected {
						style = selectedStyle
					}
					e.screen.SetContent(x, y, ' ', nil, style)
				}
			}

			// Button text
			for j, r := range opt {
				x := buttonX + 2 + j // Position text with padding
				y := buttonY + 1     // Center text vertically

				style := buttonStyle
				if i == selected {
					style = selectedStyle
				}

				if x >= dialogX && x < dialogX+dialogWidth-1 {
					e.screen.SetContent(x, y, r, nil, style)
				}
			}

			// Move to next button position
			buttonX += buttonWidth + 2
		}

		e.screen.Show()

		// Handle input
		ev := e.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyLeft:
				selected = (selected + len(options) - 1) % len(options)
			case tcell.KeyRight:
				selected = (selected + 1) % len(options)
			case tcell.KeyEnter:
				switch selected {
				case 0: // Save
					if e.filePath == "" {
						e.promptForFilename()
					} else {
						e.saveFile()
					}
					close(e.quit)
					e.screen.Fini()
					return false
				case 1: // Don't Save
					close(e.quit)
					e.screen.Fini()
					return false
				case 2: // Cancel
					return true
				}
			case tcell.KeyEscape:
				return true
			}
		}
	}
}

// showMessage displays a message at the bottom of the screen
func (e *Editor) showMessage(message string) {
	width, height := e.screen.Size()

	// Dialog dimensions
	dialogWidth := min(len(message)+8, width-4)
	dialogHeight := 5
	dialogX := (width - dialogWidth) / 2
	dialogY := (height - dialogHeight) / 2

	// Create styles
	dialogStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	borderStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogBorderColor).
		Background(e.theme.DialogBackground)

	titleStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogSelectedForeground).
		Background(e.theme.DialogButtonBackground)

	textStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	title := " Message "

	// Draw dialog
	for {
		// Draw dialog background and border
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Simple rectangle with solid background
				if y == dialogY || y == dialogY+dialogHeight-1 ||
					x == dialogX || x == dialogX+dialogWidth-1 {
					e.screen.SetContent(x, y, ' ', nil, borderStyle)
				} else {
					e.screen.SetContent(x, y, ' ', nil, dialogStyle)
				}
			}
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Write message
		msgX := dialogX + (dialogWidth-len(message))/2
		for i, r := range message {
			if msgX+i >= dialogX && msgX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(msgX+i, dialogY+2, r, nil, textStyle)
			}
		}

		e.screen.Show()

		// Wait for a key event to dismiss the message
		ev := e.screen.PollEvent()
		switch ev.(type) {
		case *tcell.EventKey:
			return
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

// ensureVisibleCursor adjusts scroll position to keep cursor in view
func (e *Editor) ensureVisibleCursor() {
	_, height := e.screen.Size()
	contentHeight := height - 1 // Leave space for status line

	// Ensure cursor position is valid
	maxY := len(e.content)
	if e.cursorY > maxY {
		e.cursorY = maxY
		e.cursorX = 0
	}

	// If cursor is above the visible area, scroll up
	if e.cursorY < e.scrollY {
		e.scrollY = e.cursorY
	}

	// If cursor is below the visible area, scroll down
	if e.cursorY >= e.scrollY+contentHeight {
		e.scrollY = e.cursorY - contentHeight + 1
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
