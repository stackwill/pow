package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Search state
	searchMode       bool
	searchQuery      string
	searchResults    []SearchResult
	currentSearchIdx int

	// Key counter for cursor movement
	keyCounter int
}

// SearchResult represents a found match
type SearchResult struct {
	Line int
	Col  int
	Len  int
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
	// Always use local config directory, make sure we check if it exists
	configPath := filepath.Join("config", "config.conf")

	// Ensure we have a valid theme even if loading fails
	var theme *config.Theme

	// Try to load the theme, handle any errors
	theme, themeErr := config.LoadTheme(configPath)
	if themeErr != nil {
		// Just print the error, don't abort - we'll use the default theme
		fmt.Fprintln(os.Stderr, "Theme loading error:", themeErr)

		// If theme is nil, create a default theme to avoid nil pointer dereference
		if theme == nil {
			theme = &config.Theme{
				BackgroundColor:  tcell.NewRGBColor(40, 44, 52),    // Dark background
				TextColor:        tcell.NewRGBColor(220, 223, 228), // Light text
				CursorColor:      tcell.NewRGBColor(255, 165, 0),   // Orange cursor
				StatusBackground: tcell.NewRGBColor(45, 50, 60),    // Darker status bar
				StatusForeground: tcell.ColorBlack,                 // Black text for status
				StatusIconColor:  tcell.NewRGBColor(147, 197, 253), // Light blue for icons

				// Default dialog colors
				DialogBackground:         tcell.NewRGBColor(40, 45, 55),    // Dark dialog bg
				DialogForeground:         tcell.NewRGBColor(230, 230, 230), // Light text
				DialogBorderColor:        tcell.NewRGBColor(80, 90, 110),   // Dark border
				DialogButtonBackground:   tcell.NewRGBColor(70, 100, 170),  // Blue button bg
				DialogButtonForeground:   tcell.NewRGBColor(240, 240, 240), // White button text
				DialogSelectedBackground: tcell.NewRGBColor(100, 140, 210), // Bright blue selection
				DialogSelectedForeground: tcell.NewRGBColor(255, 255, 255), // White selected text

				// Default icons as runes
				IconSave:       '🖫', // Save icon fallback
				IconExit:       '✕', // Exit icon fallback
				IconFind:       '🔍', // Find icon fallback
				IconFile:       '📄', // File icon fallback
				IconModified:   '●', // Modified indicator fallback
				IconPosition:   '⌖', // Position indicator fallback
				IconPercentage: '%', // Percentage fallback
			}
		}
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
		screen:           screen,
		filePath:         filePath,
		content:          content,
		theme:            theme,
		highlighter:      highlighter,
		cursorX:          0,
		cursorY:          0,
		scrollY:          0,
		modified:         !fileExists, // Mark as modified if it's a new file
		quit:             make(chan struct{}),
		searchMode:       false,
		searchQuery:      "",
		searchResults:    []SearchResult{},
		currentSearchIdx: -1,
		keyCounter:       0,
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
			if e.searchMode {
				if !e.handleSearchInput(ev) {
					e.draw()
					continue
				}
			}

			if !e.handleKeyEvent(ev) {
				return nil // Exit requested
			}

			// Only redraw for specific keys or periodically
			shouldDraw := true

			// Don't redraw for cursor navigation keys to improve speed
			if ev.Key() == tcell.KeyDown || ev.Key() == tcell.KeyUp ||
				ev.Key() == tcell.KeyLeft || ev.Key() == tcell.KeyRight {
				// Set to false to skip redraw for cursor movement, making it much faster
				shouldDraw = false
			}

			// Force redraw on Space key to let the user see current position
			if ev.Key() == tcell.KeyRune && ev.Rune() == ' ' {
				shouldDraw = true
				e.keyCounter = 0
			}

			// Periodically redraw every 20 navigation events to keep screen updated
			if !shouldDraw {
				// Get event count for this key
				e.keyCounter++
				if e.keyCounter >= 20 {
					shouldDraw = true
					e.keyCounter = 0
				}
			}

			if shouldDraw {
				e.draw()
			}
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

				// Check if we have a search result at this position
				inSearchResult := false
				if len(e.searchResults) > 0 {
					for idx, result := range e.searchResults {
						if i == result.Line && x >= result.Col && x < result.Col+result.Len {
							// Highlight search matches
							if idx == e.currentSearchIdx {
								// Current match - make it stand out more
								style = tcell.StyleDefault.
									Foreground(e.theme.DialogBackground).
									Background(e.theme.DialogSelectedBackground)
							} else {
								// Other matches
								style = tcell.StyleDefault.
									Foreground(e.theme.DialogButtonForeground).
									Background(e.theme.DialogButtonBackground)
							}
							inSearchResult = true
							break
						}
					}
				}

				// If not in a search result, use syntax highlighting
				if !inSearchResult {
					// Check if we have a highlighted segment that includes this position
					for _, segment := range colorSegments {
						if x >= segment.StartCol && x < segment.EndCol {
							// Apply the highlight style but preserve background color
							style = segment.Style.Background(e.theme.BackgroundColor)
							break
						}
					}
				}

				e.screen.SetContent(x, y, r, nil, style)
			}
		}
		// The extra line beyond content is already drawn as empty space
	}

	// Draw cursor (only if it's in the visible area)
	if e.cursorY >= e.scrollY && e.cursorY < e.scrollY+height-1 && !e.searchMode {
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

	iconStyle := tcell.StyleDefault.
		Foreground(e.theme.StatusIconColor).
		Background(e.theme.StatusBackground)

	// Fill status line with background color
	for x := 0; x < width; x++ {
		e.screen.SetContent(x, height-1, ' ', nil, statusStyle)
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
		scrollInfo = fmt.Sprintf(" %c %d-%d/%d %c %d%%",
			e.theme.IconPosition, visibleStart, visibleEnd, totalLines,
			e.theme.IconPercentage, scrollPercentage)
	}

	// Show keybindings with icons
	keybindings := fmt.Sprintf("%c:Save %c:Exit %c:Find",
		e.theme.IconSave, e.theme.IconExit, e.theme.IconFind)

	// Create the status text with icons
	statusText := fmt.Sprintf(" %c %c %s [%s] [%d:%d]%s",
		e.theme.IconModified, e.theme.IconFile, e.filePath, fileType, e.cursorY+1, e.cursorX+1, scrollInfo)

	// Draw the status text
	x := 0
	for _, r := range statusText {
		if x < width {
			// Use icon style for icons
			style := statusStyle
			if r == e.theme.IconModified || r == e.theme.IconFile ||
				r == e.theme.IconPosition || r == e.theme.IconPercentage {
				style = iconStyle
			}
			e.screen.SetContent(x, height-1, r, nil, style)
			x++
		}
	}

	// Draw keybindings on the right side
	if len(keybindings) < width {
		startX := width - len(keybindings) - 1
		for i, r := range keybindings {
			// Use icon style for icons
			style := statusStyle
			if r == e.theme.IconSave || r == e.theme.IconExit ||
				r == e.theme.IconFind {
				style = iconStyle
			}
			e.screen.SetContent(startX+i, height-1, r, nil, style)
		}
	}

	// If in search mode, draw the search input
	if e.searchMode {
		e.drawSearchInput()
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
		// If it's the default untitled file, we must prompt for a name
		if e.filePath == "untitled.txt" && !fileExists(e.filePath) {
			e.promptForFilename()
		} else {
			e.saveFile()
		}
		return true

	case tcell.KeyCtrlF: // Find
		e.enterSearchMode()
		return true

	case tcell.KeyCtrlV: // Paste
		e.pasteFromClipboard()
		return true

	case tcell.KeyUp:
		// Allow fast movement when holding Up key - move multiple lines at once
		moveAmount := 1

		// If holding key down for a while (as tracked by keyCounter), increase speed
		if e.keyCounter > 5 {
			moveAmount = 3
		}
		if e.keyCounter > 10 {
			moveAmount = 5
		}
		if e.keyCounter > 15 {
			moveAmount = 10
		}

		// Apply the movement
		newY := e.cursorY - moveAmount
		if newY < 0 {
			newY = 0 // Don't go above first line
		}

		e.cursorY = newY

		// Make sure cursorX is not beyond end of line
		if e.cursorX > len(e.content[e.cursorY]) {
			e.cursorX = len(e.content[e.cursorY])
		}

		// Update scroll position to keep cursor in view
		e.ensureVisibleCursor()
		return true

	case tcell.KeyDown:
		// Allow fast movement when holding Down key - move multiple lines at once
		maxY := len(e.content)
		moveAmount := 1

		// If holding key down for a while (as tracked by keyCounter), increase speed
		if e.keyCounter > 5 {
			moveAmount = 3
		}
		if e.keyCounter > 10 {
			moveAmount = 5
		}
		if e.keyCounter > 15 {
			moveAmount = 10
		}

		// Apply the movement
		newY := e.cursorY + moveAmount
		if newY > maxY {
			newY = maxY // Don't go beyond the extra line
		}

		e.cursorY = newY

		// Adjust cursor X if needed
		if e.cursorY < maxY && e.cursorX > len(e.content[e.cursorY]) {
			e.cursorX = len(e.content[e.cursorY])
		} else if e.cursorY == maxY {
			// We're on the extra line beyond content
			e.cursorX = 0
		}

		// Update scroll position to keep cursor in view
		e.ensureVisibleCursor()
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
		// Move cursor down by a page with no speed limitations
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
	if e.filePath == "" || e.filePath == "untitled.txt" {
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

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	// Return false for directories
	return !info.IsDir()
}

// promptForFilename asks the user for a filename to save
func (e *Editor) promptForFilename() {
	width, height := e.screen.Size()

	// Dialog dimensions
	dialogWidth := min(60, width-4)
	dialogHeight := 11 // Increased height for better spacing
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

	// Shadow style
	shadowStyle := tcell.StyleDefault.
		Background(tcell.NewRGBColor(10, 10, 10)).
		Foreground(tcell.NewRGBColor(10, 10, 10))

	// Create an input field at the bottom of the screen
	prompt := "Enter filename: "
	input := e.filePath
	if input == "untitled.txt" {
		input = ""
	}
	title := " Save File "

	// Box drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'

	// Process input until Enter or Esc is pressed
	for {
		// Draw dialog shadow first (before the dialog)
		for y := dialogY + 1; y <= dialogY+dialogHeight; y++ {
			for x := dialogX + 2; x <= dialogX+dialogWidth+1; x++ {
				if y == dialogY+dialogHeight || x == dialogX+dialogWidth+1 {
					e.screen.SetContent(x, y, ' ', nil, shadowStyle)
				}
			}
		}

		// Draw dialog background
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Fill with background
				e.screen.SetContent(x, y, ' ', nil, dialogStyle)
			}
		}

		// Draw dialog border
		// Top and bottom borders
		for x := dialogX; x < dialogX+dialogWidth; x++ {
			if x == dialogX {
				e.screen.SetContent(x, dialogY, topLeft, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomLeft, nil, borderStyle)
			} else if x == dialogX+dialogWidth-1 {
				e.screen.SetContent(x, dialogY, topRight, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomRight, nil, borderStyle)
			} else {
				e.screen.SetContent(x, dialogY, horizontal, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, horizontal, nil, borderStyle)
			}
		}

		// Left and right borders
		for y := dialogY + 1; y < dialogY+dialogHeight-1; y++ {
			e.screen.SetContent(dialogX, y, vertical, nil, borderStyle)
			e.screen.SetContent(dialogX+dialogWidth-1, y, vertical, nil, borderStyle)
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX+1 && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Draw prompt
		promptX := dialogX + 3
		for i, c := range prompt {
			e.screen.SetContent(promptX+i, dialogY+5, c, nil, inputStyle)
		}

		// Draw input
		inputX := promptX + len(prompt)
		for i, c := range input {
			if inputX+i < dialogX+dialogWidth-3 {
				e.screen.SetContent(inputX+i, dialogY+5, c, nil, inputStyle)
			}
		}

		// Draw input field border
		inputFieldWidth := dialogWidth - 6
		for x := promptX; x < promptX+inputFieldWidth; x++ {
			if i := x - inputX; i >= 0 && i < len(input) {
				continue // Skip where there's text
			}
			e.screen.SetContent(x, dialogY+5, '_', nil, inputStyle)
		}

		// Show cursor
		e.screen.SetContent(inputX+len(input), dialogY+5, ' ', nil, cursorStyle)

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
				// Only add the character if it would fit in the dialog
				if inputX+len(input) < dialogX+dialogWidth-3 {
					input += string(ev.Rune())
				}
			}
		}
	}
}

// enterSearchMode activates search mode with an input field
func (e *Editor) enterSearchMode() {
	e.searchMode = true
	e.searchQuery = ""
	e.searchResults = []SearchResult{}
	e.currentSearchIdx = -1
	e.draw()
}

// drawSearchInput displays the search input interface
func (e *Editor) drawSearchInput() {
	width, _ := e.screen.Size()

	// Input style
	inputBgStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogForeground).
		Background(e.theme.DialogBackground)

	cursorStyle := tcell.StyleDefault.
		Foreground(e.theme.DialogBackground).
		Background(e.theme.DialogSelectedBackground)

	iconStyle := tcell.StyleDefault.
		Foreground(e.theme.StatusIconColor).
		Background(e.theme.DialogBackground)

	prompt := fmt.Sprintf("%c Search: ", e.theme.IconFind)

	// Draw search bar at the top of the screen
	for x := 0; x < width; x++ {
		e.screen.SetContent(x, 0, ' ', nil, inputBgStyle)
	}

	// Draw prompt with icon
	for i, c := range prompt {
		style := inputBgStyle
		if i == 0 {
			style = iconStyle
		}
		e.screen.SetContent(i, 0, c, nil, style)
	}

	// Draw search query
	for i, c := range e.searchQuery {
		e.screen.SetContent(len(prompt)+i, 0, c, nil, inputBgStyle)
	}

	// Draw cursor
	e.screen.SetContent(len(prompt)+len(e.searchQuery), 0, ' ', nil, cursorStyle)

	// Show search count if there are results
	if len(e.searchResults) > 0 {
		countText := fmt.Sprintf(" %c %d/%d", e.theme.IconPosition, e.currentSearchIdx+1, len(e.searchResults))
		countX := len(prompt) + len(e.searchQuery) + 2

		for i, c := range countText {
			style := inputBgStyle
			if i == 0 {
				style = iconStyle
			}
			e.screen.SetContent(countX+i, 0, c, nil, style)
		}
	}
}

// handleSearchInput handles keyboard input during search mode
func (e *Editor) handleSearchInput(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape:
		e.exitSearchMode()
		return true

	case tcell.KeyEnter:
		if len(e.searchResults) > 0 {
			// Cycle to next result
			e.currentSearchIdx = (e.currentSearchIdx + 1) % len(e.searchResults)
			e.navigateToSearchResult(e.currentSearchIdx)
		}
		return false

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(e.searchQuery) > 0 {
			e.searchQuery = e.searchQuery[:len(e.searchQuery)-1]
			e.performSearch()
		}
		return false

	case tcell.KeyRune:
		e.searchQuery += string(ev.Rune())
		e.performSearch()
		return false
	}

	return true
}

// performSearch searches for query matches in the content
func (e *Editor) performSearch() {
	if e.searchQuery == "" {
		e.searchResults = []SearchResult{}
		e.currentSearchIdx = -1
		return
	}

	// Find all occurrences of the search query
	query := strings.ToLower(e.searchQuery)
	results := []SearchResult{}

	for lineIdx, line := range e.content {
		lineLower := strings.ToLower(line)
		startIdx := 0

		for {
			foundIdx := strings.Index(lineLower[startIdx:], query)
			if foundIdx == -1 {
				break
			}

			// Calculate the actual position in the line
			actualIdx := startIdx + foundIdx

			// Add this result
			results = append(results, SearchResult{
				Line: lineIdx,
				Col:  actualIdx,
				Len:  len(query),
			})

			// Move start index for next search
			startIdx = actualIdx + len(query)
		}
	}

	e.searchResults = results

	// If we have results, set the current index to the first result
	if len(results) > 0 {
		e.currentSearchIdx = 0
		e.navigateToSearchResult(0)
	} else {
		e.currentSearchIdx = -1
	}
}

// navigateToSearchResult positions the cursor at a search result
func (e *Editor) navigateToSearchResult(idx int) {
	if idx < 0 || idx >= len(e.searchResults) {
		return
	}

	result := e.searchResults[idx]
	e.cursorY = result.Line
	e.cursorX = result.Col

	// Ensure the result is visible
	e.ensureVisibleCursor()
}

// exitSearchMode returns to normal editing mode
func (e *Editor) exitSearchMode() {
	e.searchMode = false
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

// showMessage displays a message at the bottom of the screen
func (e *Editor) showMessage(message string) {
	width, height := e.screen.Size()

	// Dialog dimensions
	dialogWidth := min(len(message)+8, width-4)
	dialogHeight := 7 // Increased for better spacing
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

	// Shadow style
	shadowStyle := tcell.StyleDefault.
		Background(tcell.NewRGBColor(10, 10, 10)).
		Foreground(tcell.NewRGBColor(10, 10, 10))

	// Box drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'

	title := " Message "

	// Draw dialog
	for {
		// Draw dialog shadow first
		for y := dialogY + 1; y <= dialogY+dialogHeight; y++ {
			for x := dialogX + 2; x <= dialogX+dialogWidth+1; x++ {
				if y == dialogY+dialogHeight || x == dialogX+dialogWidth+1 {
					e.screen.SetContent(x, y, ' ', nil, shadowStyle)
				}
			}
		}

		// Draw dialog background
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Fill with background
				e.screen.SetContent(x, y, ' ', nil, dialogStyle)
			}
		}

		// Draw dialog border
		// Top and bottom borders
		for x := dialogX; x < dialogX+dialogWidth; x++ {
			if x == dialogX {
				e.screen.SetContent(x, dialogY, topLeft, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomLeft, nil, borderStyle)
			} else if x == dialogX+dialogWidth-1 {
				e.screen.SetContent(x, dialogY, topRight, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomRight, nil, borderStyle)
			} else {
				e.screen.SetContent(x, dialogY, horizontal, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, horizontal, nil, borderStyle)
			}
		}

		// Left and right borders
		for y := dialogY + 1; y < dialogY+dialogHeight-1; y++ {
			e.screen.SetContent(dialogX, y, vertical, nil, borderStyle)
			e.screen.SetContent(dialogX+dialogWidth-1, y, vertical, nil, borderStyle)
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX+1 && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Write message
		msgX := dialogX + (dialogWidth-len(message))/2
		for i, r := range message {
			if msgX+i >= dialogX+1 && msgX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(msgX+i, dialogY+3, r, nil, textStyle)
			}
		}

		// Draw a hint at the bottom
		hint := "Press any key to continue"
		hintX := dialogX + (dialogWidth-len(hint))/2
		for i, r := range hint {
			if hintX+i >= dialogX+1 && hintX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(hintX+i, dialogY+dialogHeight-2, r, nil, textStyle)
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

// promptSaveBeforeExit asks the user if they want to save before exiting
func (e *Editor) promptSaveBeforeExit() bool {
	width, height := e.screen.Size()

	// Options
	options := []string{"Save", "Don't Save", "Cancel"}
	selected := 0

	message := "Save changes before exiting?"

	// Dialog dimensions
	dialogWidth := min(50, width-4)
	dialogHeight := 9 // Increased for better spacing
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

	// Shadow style
	shadowStyle := tcell.StyleDefault.
		Background(tcell.NewRGBColor(10, 10, 10)).
		Foreground(tcell.NewRGBColor(10, 10, 10))

	// Box drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'

	title := " Confirm Exit "

	for {
		// Draw dialog shadow first
		for y := dialogY + 1; y <= dialogY+dialogHeight; y++ {
			for x := dialogX + 2; x <= dialogX+dialogWidth+1; x++ {
				if y == dialogY+dialogHeight || x == dialogX+dialogWidth+1 {
					e.screen.SetContent(x, y, ' ', nil, shadowStyle)
				}
			}
		}

		// Draw dialog background
		for y := dialogY; y < dialogY+dialogHeight; y++ {
			for x := dialogX; x < dialogX+dialogWidth; x++ {
				// Fill with background
				e.screen.SetContent(x, y, ' ', nil, dialogStyle)
			}
		}

		// Draw dialog border
		// Top and bottom borders
		for x := dialogX; x < dialogX+dialogWidth; x++ {
			if x == dialogX {
				e.screen.SetContent(x, dialogY, topLeft, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomLeft, nil, borderStyle)
			} else if x == dialogX+dialogWidth-1 {
				e.screen.SetContent(x, dialogY, topRight, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, bottomRight, nil, borderStyle)
			} else {
				e.screen.SetContent(x, dialogY, horizontal, nil, borderStyle)
				e.screen.SetContent(x, dialogY+dialogHeight-1, horizontal, nil, borderStyle)
			}
		}

		// Left and right borders
		for y := dialogY + 1; y < dialogY+dialogHeight-1; y++ {
			e.screen.SetContent(dialogX, y, vertical, nil, borderStyle)
			e.screen.SetContent(dialogX+dialogWidth-1, y, vertical, nil, borderStyle)
		}

		// Draw title
		titleX := dialogX + (dialogWidth-len(title))/2
		for i, c := range title {
			if titleX+i >= dialogX+1 && titleX+i < dialogX+dialogWidth-1 {
				e.screen.SetContent(titleX+i, dialogY, c, nil, titleStyle)
			}
		}

		// Draw message
		for i, r := range message {
			x := dialogX + (dialogWidth-len(message))/2 + i
			y := dialogY + 3
			if x >= dialogX+1 && x < dialogX+dialogWidth-1 {
				e.screen.SetContent(x, y, r, nil, textStyle)
			}
		}

		// Draw buttons
		buttonY := dialogY + 6

		// Calculate total width of all buttons with spacing
		totalButtonWidth := 0
		for _, opt := range options {
			totalButtonWidth += len(opt) + 4 // Add padding around button text
		}
		totalButtonWidth += (len(options) - 1) * 3 // More spacing between buttons

		// Start position for first button
		buttonX := dialogX + (dialogWidth-totalButtonWidth)/2

		for i, opt := range options {
			// Draw button with rounded corners
			buttonWidth := len(opt) + 4

			// Button style based on selection
			style := buttonStyle
			if i == selected {
				style = selectedStyle
			}

			// Button border and background
			// Top border with rounded corners
			e.screen.SetContent(buttonX, buttonY, '╭', nil, style)
			e.screen.SetContent(buttonX+buttonWidth-1, buttonY, '╮', nil, style)

			// Fill top row
			for x := buttonX + 1; x < buttonX+buttonWidth-1; x++ {
				e.screen.SetContent(x, buttonY, '─', nil, style)
			}

			// Middle row with text
			e.screen.SetContent(buttonX, buttonY+1, '│', nil, style)
			e.screen.SetContent(buttonX+buttonWidth-1, buttonY+1, '│', nil, style)

			// Fill middle row
			for x := buttonX + 1; x < buttonX+buttonWidth-1; x++ {
				e.screen.SetContent(x, buttonY+1, ' ', nil, style)
			}

			// Bottom border with rounded corners
			e.screen.SetContent(buttonX, buttonY+2, '╰', nil, style)
			e.screen.SetContent(buttonX+buttonWidth-1, buttonY+2, '╯', nil, style)

			// Fill bottom row
			for x := buttonX + 1; x < buttonX+buttonWidth-1; x++ {
				e.screen.SetContent(x, buttonY+2, '─', nil, style)
			}

			// Button text
			for j, r := range opt {
				x := buttonX + 2 + j // Position text with padding
				y := buttonY + 1     // Center text vertically

				if x >= dialogX+1 && x < dialogX+dialogWidth-1 {
					e.screen.SetContent(x, y, r, nil, style)
				}
			}

			// Move to next button position
			buttonX += buttonWidth + 3
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
					if e.filePath == "untitled.txt" && !fileExists(e.filePath) {
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

// pasteFromClipboard implements paste functionality
func (e *Editor) pasteFromClipboard() {
	// Get clipboard content from the terminal
	// This is a simplified implementation that assumes the system has xclip or pbpaste
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	default:
		// Unsupported platform
		return
	}

	out, err := cmd.Output()
	if err != nil {
		return // Failed to get clipboard content
	}

	// Get the content as string and split by lines
	content := string(out)
	lines := strings.Split(content, "\n")

	// If there's no content, do nothing
	if len(content) == 0 {
		return
	}

	// Handle multi-line paste more efficiently
	if len(lines) > 1 {
		e.insertMultiLineText(lines)
	} else {
		// Single line paste
		e.insertTextAtCursor(content)
	}

	e.modified = true
}

// insertMultiLineText inserts multiple lines of text efficiently
func (e *Editor) insertMultiLineText(lines []string) {
	// Handle the first line - append to current line at cursor position
	currentLine := e.content[e.cursorY]
	leftPart := currentLine[:e.cursorX]
	rightPart := ""
	if e.cursorX < len(currentLine) {
		rightPart = currentLine[e.cursorX:]
	}

	// Update first line
	newFirstLine := leftPart + lines[0]

	// Create a new slice to hold all content
	newContent := make([]string, len(e.content)+len(lines)-1)

	// Copy content before the cursor line
	copy(newContent, e.content[:e.cursorY])

	// Add the modified first line
	newContent[e.cursorY] = newFirstLine

	// Add all middle lines
	for i := 1; i < len(lines)-1; i++ {
		newContent[e.cursorY+i] = lines[i]
	}

	// Handle the last line + right part of split line
	if len(lines) > 1 {
		lastIdx := len(lines) - 1
		newContent[e.cursorY+lastIdx] = lines[lastIdx] + rightPart

		// Move cursor to the end of the last inserted line
		e.cursorY += lastIdx
		e.cursorX = len(lines[lastIdx])
	} else {
		// Only one line was pasted, cursor should be after the inserted text
		e.cursorX += len(lines[0])
	}

	// Copy content after the cursor line
	copy(newContent[e.cursorY+1:], e.content[e.cursorY+1:])

	// Update content
	e.content = newContent
}

// insertTextAtCursor inserts a single line of text at the cursor position
func (e *Editor) insertTextAtCursor(text string) {
	// Insert text at cursor position
	currentLine := e.content[e.cursorY]
	if e.cursorX > len(currentLine) {
		// Pad with spaces if cursor is beyond the end of the line
		e.content[e.cursorY] = currentLine + strings.Repeat(" ", e.cursorX-len(currentLine)) + text
	} else {
		e.content[e.cursorY] = currentLine[:e.cursorX] + text + currentLine[e.cursorX:]
	}

	// Move cursor after inserted text
	e.cursorX += len(text)
}
