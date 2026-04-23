package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/c-bata/go-prompt"
	"golang.org/x/term"
)

type terminalSizeFunc func() (rows, cols int, err error)

type statusLineWriter struct {
	prompt.ConsoleWriter
	text statusTextFunc
	size terminalSizeFunc

	rows     int
	cols     int
	attached bool
}

type statusTextFunc func() string

func newStatusLineWriter(out prompt.ConsoleWriter, text string) *statusLineWriter {
	return newDynamicStatusLineWriter(out, func() string {
		return text
	})
}

func newDynamicStatusLineWriter(out prompt.ConsoleWriter, text statusTextFunc) *statusLineWriter {
	return &statusLineWriter{
		ConsoleWriter: out,
		text:          text,
		size:          stdoutTerminalSize,
	}
}

func stdoutTerminalSize() (rows, cols int, err error) {
	cols, rows, err = term.GetSize(int(os.Stdout.Fd()))
	return rows, cols, err
}

func (w *statusLineWriter) Attach() {
	if w.attached || !w.refreshSize() {
		return
	}
	w.attached = true

	w.HideCursor()
	w.renderStatusLine()
	w.setScrollRegion()
	w.ConsoleWriter.CursorGoTo(2, 1)
	w.ShowCursor()
	_ = w.ConsoleWriter.Flush()
}

func (w *statusLineWriter) Close() {
	if !w.attached {
		return
	}

	w.HideCursor()
	w.ConsoleWriter.WriteRawStr("\x1b[r")
	w.ConsoleWriter.CursorGoTo(0, 0)
	w.EraseLine()
	w.ConsoleWriter.CursorGoTo(0, 0)
	w.ShowCursor()
	_ = w.ConsoleWriter.Flush()
	w.attached = false
}

func (w *statusLineWriter) Flush() error {
	if w.attached {
		w.refreshSize()
		w.renderStatusLine()
	}
	return w.ConsoleWriter.Flush()
}

func (w *statusLineWriter) EraseScreen() {
	if !w.attached {
		w.ConsoleWriter.EraseScreen()
		return
	}

	w.ConsoleWriter.SaveCursor()
	w.ConsoleWriter.CursorGoTo(2, 1)
	w.ConsoleWriter.EraseDown()
	w.ConsoleWriter.UnSaveCursor()
	w.renderStatusLine()
}

func (w *statusLineWriter) CursorGoTo(row, col int) {
	if !w.attached {
		w.ConsoleWriter.CursorGoTo(row, col)
		return
	}
	if row == 0 && col == 0 {
		w.ConsoleWriter.CursorGoTo(2, 1)
		return
	}
	w.ConsoleWriter.CursorGoTo(row+1, col)
}

func (w *statusLineWriter) refreshSize() bool {
	rows, cols, err := w.size()
	if err != nil || rows < 2 || cols < 1 {
		return false
	}
	changed := rows != w.rows || cols != w.cols
	w.rows = rows
	w.cols = cols
	if changed && w.attached {
		w.ConsoleWriter.SaveCursor()
		w.setScrollRegion()
		w.ConsoleWriter.UnSaveCursor()
	}
	return true
}

func (w *statusLineWriter) setScrollRegion() {
	w.ConsoleWriter.WriteRawStr("\x1b[2;" + strconv.Itoa(w.rows) + "r")
}

func (w *statusLineWriter) renderStatusLine() {
	w.ConsoleWriter.SaveCursor()
	w.ConsoleWriter.CursorGoTo(0, 0)
	w.ConsoleWriter.WriteRawStr("\x1b[7m")
	w.ConsoleWriter.WriteStr(formatStatusLine(w.text(), w.cols))
	w.ConsoleWriter.WriteRawStr("\x1b[0m")
	w.ConsoleWriter.UnSaveCursor()
}

func formatStatusLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) > width {
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-len(runes))
}

type statusLineParser struct {
	prompt.ConsoleParser
}

func newStatusLineParser(parser prompt.ConsoleParser) *statusLineParser {
	return &statusLineParser{ConsoleParser: parser}
}

func (p *statusLineParser) GetWinSize() *prompt.WinSize {
	size := p.ConsoleParser.GetWinSize()
	if size != nil && size.Row > 1 {
		return &prompt.WinSize{Row: size.Row - 1, Col: size.Col}
	}
	return size
}
