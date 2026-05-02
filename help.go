package main

import (
	"fmt"
	"io"
)

func printStartupMessage(w io.Writer) {
	fmt.Fprintln(w, versionString())
	fmt.Fprintln(w, "Please use `exit` or `Ctrl-D` to exit this program.")
	fmt.Fprintln(w, "Type `/help` for shortcuts and prompt commands.")
}

func cheatSheetText() string {
	return `kube-prompt cheat sheet

Ctrl shortcuts:
  Ctrl-A  Move to the beginning of the line
  Ctrl-E  Move to the end of the line
  Ctrl-F  Move forward one character
  Ctrl-B  Move backward one character
  Ctrl-P  Previous command in history
  Ctrl-N  Next command in history
  Ctrl-H  Delete the character before the cursor
  Ctrl-W  Delete the word before the cursor
  Ctrl-U  Delete from the cursor to the start of the line
  Ctrl-K  Delete from the cursor to the end of the line
  Ctrl-L  Clear the screen
  Ctrl-D  Exit when the input is empty
  Ctrl-S  Open output history / scroll mode when output exists

Prompt commands:
  /help                          Show this cheat sheet
  /namespace [NAME]              Show or change the active namespace
  /outputs [latest|INDEX|id:ID]  Browse captured command output
  /output save all|last|ID PATH  Save captured output to a file
  /exit                          Exit kube-prompt
`
}
