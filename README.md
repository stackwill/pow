# Pow

A simple TUI text editor

Lots of inspiration for Design comes from GNU Nano, the intention is to make a Nano-like experience with syntax highlighting, user theming and other basic improvements.

## Features

- **Theming Engine**
- Text navigation
- Text editing
- Syntax highlighting
- Save functionality
- Search

# Theming

Set the theme of the editor in the config/config.conf file
Themes are stored in the config/themes/ directory, where there are already theme templates to build on or use.

## Run

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



## Development

Cursor was heavily used in the development of Pow. The main model used was Claude 3.7.
