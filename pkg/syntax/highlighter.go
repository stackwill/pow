package syntax

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v2"
)

// ColoredLine represents a single line of syntax-highlighted text
type ColoredLine struct {
	Text   string
	Colors []ColorSegment
}

// ColorSegment represents a segment of text with a specific color
type ColorSegment struct {
	StartCol int
	EndCol   int
	Style    tcell.Style
}

// Highlighter manages syntax highlighting
type Highlighter struct {
	lexer     chroma.Lexer
	formatter chroma.Formatter
	style     *chroma.Style
}

// NewHighlighter creates a new syntax highlighter for the specified file
func NewHighlighter(filePath string) *Highlighter {
	// Determine lexer based on file extension
	var lexer chroma.Lexer

	// Try to match by file extension
	lexer = lexers.Match(filePath)
	if lexer == nil {
		// Try to match by filename
		lexer = lexers.Match(filepath.Base(filePath))
	}

	// Default to plaintext if no lexer found
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Use a coalescing lexer to improve performance
	lexer = chroma.Coalesce(lexer)

	// Get a suitable style for syntax highlighting (default to "monokai")
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	// Use the NoOp formatter as we'll handle the rendering ourselves
	formatter := formatters.Get("noop")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	return &Highlighter{
		lexer:     lexer,
		formatter: formatter,
		style:     style,
	}
}

// HighlightContent highlights the content of a file
func (h *Highlighter) HighlightContent(content string) []ColoredLine {
	// Split content into lines for processing
	lines := strings.Split(content, "\n")
	result := make([]ColoredLine, len(lines))

	// Tokenize the entire content
	iterator, err := h.lexer.Tokenise(nil, content)
	if err != nil {
		// On error, just return the plain text without highlighting
		for i, line := range lines {
			result[i] = ColoredLine{
				Text:   line,
				Colors: []ColorSegment{},
			}
		}
		return result
	}

	// Track current line and token positions
	currentLineIdx := 0
	startPos := 0

	// Process each token from the lexer
	for token := iterator(); token != chroma.EOF; token = iterator() {
		// Get the style for this token
		tokenStyle := h.style.Get(token.Type)

		// Skip tokens with no foreground color
		if tokenStyle.Colour == 0 {
			startPos += len(token.Value)
			continue
		}

		// Convert token style to tcell style
		tcellStyle := chromaStyleToTcellStyle(tokenStyle)

		// Handle multi-line tokens
		tokenLines := strings.Split(token.Value, "\n")
		for i, tokenLine := range tokenLines {
			if i > 0 {
				// Move to the next line
				currentLineIdx++
				startPos = 0
			}

			if currentLineIdx >= len(lines) {
				// We've gone past the end of the input, which shouldn't happen
				break
			}

			// Calculate token position for this line
			startCol := startPos
			endCol := startCol + len(tokenLine)

			// Add this segment to the current line
			if len(tokenLine) > 0 {
				if result[currentLineIdx].Text == "" {
					result[currentLineIdx].Text = lines[currentLineIdx]
				}

				result[currentLineIdx].Colors = append(
					result[currentLineIdx].Colors,
					ColorSegment{
						StartCol: startCol,
						EndCol:   endCol,
						Style:    tcellStyle,
					},
				)
			}

			startPos += len(tokenLine)
		}
	}

	// Ensure all lines have text set
	for i, line := range result {
		if line.Text == "" {
			result[i].Text = lines[i]
		}
	}

	return result
}

// HighlightLine highlights a single line of text
func (h *Highlighter) HighlightLine(line string) ColoredLine {
	result := ColoredLine{
		Text:   line,
		Colors: []ColorSegment{},
	}

	// For a single line, it's more efficient to highlight the whole content
	// and extract just the line we need
	iterator, err := h.lexer.Tokenise(nil, line)
	if err != nil {
		return result
	}

	startPos := 0

	// Process tokens
	for token := iterator(); token != chroma.EOF; token = iterator() {
		// Get the style for this token
		tokenStyle := h.style.Get(token.Type)

		// Skip tokens with no foreground color
		if tokenStyle.Colour == 0 {
			startPos += len(token.Value)
			continue
		}

		// Convert token style to tcell style
		tcellStyle := chromaStyleToTcellStyle(tokenStyle)

		// Handle token (assume no newlines in a single line)
		startCol := startPos
		endCol := startCol + len(token.Value)

		// Add this segment
		if len(token.Value) > 0 {
			result.Colors = append(
				result.Colors,
				ColorSegment{
					StartCol: startCol,
					EndCol:   endCol,
					Style:    tcellStyle,
				},
			)
		}

		startPos += len(token.Value)
	}

	return result
}

// GetFileType returns the detected file type name
func (h *Highlighter) GetFileType() string {
	if h.lexer == nil {
		return "plaintext"
	}
	return h.lexer.Config().Name
}

// chromaStyleToTcellStyle converts a Chroma style to a tcell Style
func chromaStyleToTcellStyle(style chroma.StyleEntry) tcell.Style {
	// Default tcell style
	tcellStyle := tcell.StyleDefault

	// Convert the color if it exists
	if style.Colour != 0 {
		// Chroma color is a hex value like 0xRRGGBB
		// We need to convert it to individual RGB components
		hexStr := style.Colour.String()

		// Remove the leading '#' if present
		if strings.HasPrefix(hexStr, "#") {
			hexStr = hexStr[1:]
		}

		// Parse the hex color
		if rgb, err := strconv.ParseUint(hexStr, 16, 32); err == nil {
			r := int32((rgb >> 16) & 0xFF)
			g := int32((rgb >> 8) & 0xFF)
			b := int32(rgb & 0xFF)

			tcellStyle = tcellStyle.Foreground(tcell.NewRGBColor(r, g, b))
		}
	}

	// Apply font styles if needed (bold, italic, etc.)
	if style.Bold == chroma.Yes {
		tcellStyle = tcellStyle.Bold(true)
	}

	if style.Italic == chroma.Yes {
		tcellStyle = tcellStyle.Italic(true)
	}

	if style.Underline == chroma.Yes {
		tcellStyle = tcellStyle.Underline(true)
	}

	return tcellStyle
}
