package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// ThemeError represents an error that occurred while parsing the theme config
type ThemeError struct {
	ConfigPath string
	LineNum    int
	LineText   string
	Err        error
}

// Error returns a formatted error message
func (e *ThemeError) Error() string {
	if e.LineNum > 0 {
		return fmt.Sprintf("Error in %s line %d: %s - %v",
			filepath.Base(e.ConfigPath), e.LineNum, e.LineText, e.Err)
	}
	return fmt.Sprintf("Error in %s: %v", filepath.Base(e.ConfigPath), e.Err)
}

// Theme holds the color configuration for the editor
type Theme struct {
	// Main editor colors
	BackgroundColor tcell.Color
	TextColor       tcell.Color
	CursorColor     tcell.Color

	// Status line colors
	StatusBackground tcell.Color
	StatusForeground tcell.Color
}

// LoadTheme loads color configuration from the specified file
func LoadTheme(configPath string) (*Theme, error) {
	// Default theme (fallback)
	theme := &Theme{
		BackgroundColor:  tcell.NewRGBColor(40, 44, 52),    // Dark background
		TextColor:        tcell.NewRGBColor(220, 223, 228), // Light text
		CursorColor:      tcell.NewRGBColor(255, 165, 0),   // Orange cursor
		StatusBackground: tcell.NewRGBColor(110, 118, 129), // Medium gray status bar
		StatusForeground: tcell.ColorBlack,                 // Black text for status
	}

	// Try to open the config file
	file, err := os.Open(configPath)
	if err != nil {
		// Check if it's just that the file doesn't exist
		if os.IsNotExist(err) {
			return theme, &ThemeError{
				ConfigPath: configPath,
				Err:        fmt.Errorf("theme file not found, using defaults"),
			}
		}
		return theme, &ThemeError{
			ConfigPath: configPath,
			Err:        fmt.Errorf("failed to open theme file: %w", err),
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	lineText := ""

	for scanner.Scan() {
		lineNum++
		lineText = scanner.Text()
		line := strings.TrimSpace(lineText)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse color setting (key = value)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, &ThemeError{
				ConfigPath: configPath,
				LineNum:    lineNum,
				LineText:   lineText,
				Err:        fmt.Errorf("invalid syntax, expected 'key = value'"),
			}
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check for empty key/value
		if key == "" {
			return nil, &ThemeError{
				ConfigPath: configPath,
				LineNum:    lineNum,
				LineText:   lineText,
				Err:        fmt.Errorf("empty key"),
			}
		}
		if value == "" {
			return nil, &ThemeError{
				ConfigPath: configPath,
				LineNum:    lineNum,
				LineText:   lineText,
				Err:        fmt.Errorf("empty value"),
			}
		}

		// Process color values
		var color tcell.Color
		var parseErr error

		switch key {
		case "background_color", "text_color", "cursor_color", "status_background", "status_foreground":
			color, parseErr = parseRGBColor(value)
			if parseErr != nil {
				return nil, &ThemeError{
					ConfigPath: configPath,
					LineNum:    lineNum,
					LineText:   lineText,
					Err:        fmt.Errorf("invalid color value: %w", parseErr),
				}
			}

			// Assign the color to the appropriate theme field
			switch key {
			case "background_color":
				theme.BackgroundColor = color
			case "text_color":
				theme.TextColor = color
			case "cursor_color":
				theme.CursorColor = color
			case "status_background":
				theme.StatusBackground = color
			case "status_foreground":
				theme.StatusForeground = color
			}
		default:
			return nil, &ThemeError{
				ConfigPath: configPath,
				LineNum:    lineNum,
				LineText:   lineText,
				Err:        fmt.Errorf("unknown setting: %s", key),
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return nil, &ThemeError{
			ConfigPath: configPath,
			Err:        fmt.Errorf("error reading config file: %w", err),
		}
	}

	return theme, nil
}

// parseRGBColor parses an RGB color string in the format "r,g,b"
func parseRGBColor(s string) (tcell.Color, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return tcell.ColorDefault, fmt.Errorf("invalid RGB format, expected 'r,g,b', got '%s'", s)
	}

	var rgb [3]int
	for i, p := range parts {
		val, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return tcell.ColorDefault, fmt.Errorf("component %d: %w", i+1, err)
		}
		if val < 0 || val > 255 {
			return tcell.ColorDefault, fmt.Errorf("component %d: value must be between 0-255, got %d", i+1, val)
		}
		rgb[i] = val
	}

	return tcell.NewRGBColor(int32(rgb[0]), int32(rgb[1]), int32(rgb[2])), nil
}
