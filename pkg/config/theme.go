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
	StatusIconColor  tcell.Color

	// Dialog colors
	DialogBackground         tcell.Color
	DialogForeground         tcell.Color
	DialogBorderColor        tcell.Color
	DialogButtonBackground   tcell.Color
	DialogButtonForeground   tcell.Color
	DialogSelectedBackground tcell.Color
	DialogSelectedForeground tcell.Color

	// Icons - using runes for better character handling
	IconSave       rune
	IconExit       rune
	IconFind       rune
	IconFile       rune
	IconModified   rune
	IconPosition   rune
	IconPercentage rune
}

// LoadTheme loads color configuration from the specified file
func LoadTheme(configPath string) (*Theme, error) {
	// Create default theme first (fallback)
	theme := &Theme{
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

		// Default icons
		IconSave:       '󰆓',
		IconExit:       '󰅚',
		IconFind:       '󰍉',
		IconFile:       '󰈔',
		IconModified:   '󰆓',
		IconPosition:   '󰦪',
		IconPercentage: '󰎚',
	}

	// Get the theme filename from the main config
	themePath, err := getThemePathFromConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		// If we can't read the config, use the default theme path
		// Ensure we use a path relative to the application
		themePath = filepath.Join("config", "themes", "theme.conf")
	}

	// Try to open the theme file
	file, err := os.Open(themePath)
	if err != nil {
		// Check if it's just that the file doesn't exist
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Theme file not found at '%s', using defaults\n", themePath)
			return theme, nil
		}
		fmt.Fprintf(os.Stderr, "Failed to open theme file '%s', using defaults: %v\n", themePath, err)
		return theme, nil
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
			fmt.Fprintf(os.Stderr, "Invalid syntax in theme file '%s' line %d, expected 'key = value'\n", themePath, lineNum)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Handle icon settings
		if strings.HasPrefix(key, "icon_") {
			// Process single rune icon
			iconRune := []rune(value)[0]
			switch key {
			case "icon_save":
				theme.IconSave = iconRune
			case "icon_exit":
				theme.IconExit = iconRune
			case "icon_find":
				theme.IconFind = iconRune
			case "icon_file":
				theme.IconFile = iconRune
			case "icon_modified":
				theme.IconModified = iconRune
			case "icon_position":
				theme.IconPosition = iconRune
			case "icon_percentage":
				theme.IconPercentage = iconRune
			}
			continue
		}

		// Parse the color value
		var color tcell.Color
		var parseErr error

		if strings.Contains(value, ",") {
			// RGB format (r,g,b)
			color, parseErr = parseRGBColor(value)
		} else {
			// Try to interpret as a named color
			color, parseErr = parseNamedColor(value)
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Invalid color value in theme file '%s' line %d: %v\n", themePath, lineNum, parseErr)
			continue
		}

		// Assign color to the correct field
		switch key {
		case "background":
			theme.BackgroundColor = color
		case "text":
			theme.TextColor = color
		case "cursor":
			theme.CursorColor = color
		case "status_bg":
			theme.StatusBackground = color
		case "status_fg":
			theme.StatusForeground = color
		case "status_icon":
			theme.StatusIconColor = color
		case "dialog_bg":
			theme.DialogBackground = color
		case "dialog_fg":
			theme.DialogForeground = color
		case "dialog_border":
			theme.DialogBorderColor = color
		case "dialog_button_bg":
			theme.DialogButtonBackground = color
		case "dialog_button_fg":
			theme.DialogButtonForeground = color
		case "dialog_selected_bg":
			theme.DialogSelectedBackground = color
		case "dialog_selected_fg":
			theme.DialogSelectedForeground = color
		default:
			fmt.Fprintf(os.Stderr, "Unknown color setting in theme file '%s' line %d: %s\n", themePath, lineNum, key)
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading theme file '%s': %v\n", themePath, err)
	}

	return theme, nil
}

// getThemePathFromConfig reads the main config file to determine which theme to use
func getThemePathFromConfig(configPath string) (string, error) {
	// Always use paths relative to the application
	configDir := "config"

	// Path to the main config file - always use local config.conf
	mainConfigPath := filepath.Join(configDir, "config.conf")

	// Default theme path
	defaultThemePath := filepath.Join(configDir, "themes", "theme.conf")

	// Check if the main config file exists
	if _, err := os.Stat(mainConfigPath); os.IsNotExist(err) {
		return defaultThemePath, fmt.Errorf("config file not found at '%s', using default theme", mainConfigPath)
	}

	// Open the config file
	file, err := os.Open(mainConfigPath)
	if err != nil {
		return defaultThemePath, fmt.Errorf("failed to open config file '%s': %w", mainConfigPath, err)
	}
	defer file.Close()

	// Read the config file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse settings (key = value)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip invalid lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Look for the theme setting
		if key == "theme" {
			// Build the path to the theme file, always using local config
			themePath := filepath.Join(configDir, "themes", value)

			// Verify the theme file exists
			if _, err := os.Stat(themePath); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Theme file '%s' not found, falling back to default\n", themePath)
				return defaultThemePath, nil
			}

			return themePath, nil
		}
	}

	// If no theme setting found, return the default
	return defaultThemePath, nil
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

// parseNamedColor tries to parse a color name into a tcell.Color
func parseNamedColor(s string) (tcell.Color, error) {
	s = strings.ToLower(s)

	// Map of basic color names to tcell colors
	colorMap := map[string]tcell.Color{
		"black":   tcell.ColorBlack,
		"red":     tcell.ColorRed,
		"green":   tcell.ColorGreen,
		"yellow":  tcell.ColorYellow,
		"blue":    tcell.ColorBlue,
		"magenta": tcell.ColorDarkMagenta,
		"cyan":    tcell.ColorDarkCyan,
		"white":   tcell.ColorWhite,
		"gray":    tcell.ColorDarkGray,
		"grey":    tcell.ColorDarkGray,
		"orange":  tcell.NewRGBColor(255, 165, 0),
		"purple":  tcell.NewRGBColor(128, 0, 128),
	}

	if color, ok := colorMap[s]; ok {
		return color, nil
	}

	// Try to parse as hex color like "#FF0000" or "FF0000"
	if strings.HasPrefix(s, "#") {
		s = s[1:]
	}

	if len(s) == 6 {
		val, err := strconv.ParseUint(s, 16, 32)
		if err == nil {
			r := int32((val >> 16) & 0xFF)
			g := int32((val >> 8) & 0xFF)
			b := int32(val & 0xFF)
			return tcell.NewRGBColor(r, g, b), nil
		}
	}

	return tcell.ColorDefault, fmt.Errorf("unknown color name: %s", s)
}
