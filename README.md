# Pow Editor (TUI)

A simple text editor with Terminal User Interface (TUI) written in Go.

## Features (Planned)

- File loading and display (implemented)
- Text navigation
- Text editing
- Syntax highlighting
- Save functionality
- Search and replace

## Usage

```bash
go run main.go <filename>
```

Or after building:
```bash
./pow <filename>
```

For example:
```bash
./pow test.txt
```

## Controls

- Ctrl+C: Exit the editor

## Dependencies

- [tcell](https://github.com/gdamore/tcell) - Terminal handling
- [tview](https://github.com/rivo/tview) - Terminal UI widgets

## Development

This is a basic framework for a text editor. More features will be added in the future. 