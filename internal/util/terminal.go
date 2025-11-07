package util

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal checks if the given file descriptor is a terminal
func IsTerminal(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

// GetTerminalWidth returns the width of the terminal, or 80 if not a terminal
func GetTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Default width
	}
	return width
}
