package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
)

func TestShouldManageCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "get pods", want: true},
		{input: "describe pod api", want: true},
		{input: "logs deployment/api -f", want: true},
		{input: "top pod", want: true},
		{input: "exec -it api -- sh", want: false},
		{input: "attach api", want: false},
		{input: "edit deployment api", want: false},
		{input: "port-forward svc/api 8080:80", want: false},
		{input: "proxy", want: false},
		{input: "cp pod:/tmp/file ./file", want: true},
		{input: "run shell -it --image busybox", want: false},
		{input: "get pods | grep api", want: true},
		{input: "get pods > pods.txt", want: true},
		{input: "apply -f -", want: false},
		{input: "create --filename=-", want: false},
		{input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shouldManageCommand(tt.input); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestIsStreamingCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "get nodes", want: false},
		{input: "get nodes -w", want: true},
		{input: "get pods --watch", want: true},
		{input: "get pods --watch=false", want: false},
		{input: "logs pod/api -f", want: true},
		{input: "logs pod/api --follow", want: true},
		{input: "logs pod/api --follow=false", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isStreamingCommand(tt.input); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestHandleScrollKey(t *testing.T) {
	tests := []struct {
		name        string
		key         prompt.Key
		topLine     int
		totalLines  int
		visibleRows int
		want        int
	}{
		{name: "up", key: prompt.Up, topLine: 5, totalLines: 20, visibleRows: 5, want: 4},
		{name: "up clamps", key: prompt.Up, topLine: 0, totalLines: 20, visibleRows: 5, want: 0},
		{name: "down", key: prompt.Down, topLine: 5, totalLines: 20, visibleRows: 5, want: 6},
		{name: "down clamps", key: prompt.Down, topLine: 15, totalLines: 20, visibleRows: 5, want: 15},
		{name: "page up", key: prompt.PageUp, topLine: 10, totalLines: 30, visibleRows: 7, want: 3},
		{name: "page down", key: prompt.PageDown, topLine: 10, totalLines: 30, visibleRows: 7, want: 17},
		{name: "home", key: prompt.Home, topLine: 10, totalLines: 30, visibleRows: 7, want: 0},
		{name: "end", key: prompt.End, topLine: 10, totalLines: 30, visibleRows: 7, want: 23},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleScrollKey(prompt.KeyEvent{Key: tt.key}, tt.topLine, tt.totalLines, tt.visibleRows)
			if got != tt.want {
				t.Fatalf("expected top line %d, got %d", tt.want, got)
			}
		})
	}
}

func TestOutputModeStatusSuffix(t *testing.T) {
	status := &outputModeStatus{}
	if got := status.Suffix(); got != "" {
		t.Fatalf("expected empty suffix, got %q", got)
	}

	status.SetScroll(3, 10)
	if got, want := status.Suffix(), " | SCROLL line 3/10"; got != want {
		t.Fatalf("expected suffix %q, got %q", want, got)
	}
	if got, want := status.Label(), "SCROLL line 3/10"; got != want {
		t.Fatalf("expected label %q, got %q", want, got)
	}

	status.Clear()
	if got := status.Suffix(); got != "" {
		t.Fatalf("expected empty suffix after clear, got %q", got)
	}
}

func TestTerminalOutputWriterConvertsNewlines(t *testing.T) {
	var out bytes.Buffer
	writer := &terminalOutputWriter{w: &out}

	if _, err := writer.Write([]byte("one\ntwo\r\nthree")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := writer.Write([]byte("\nfour")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if got, want := out.String(), "one\r\ntwo\r\nthree\r\nfour"; got != want {
		t.Fatalf("expected converted output %q, got %q", want, got)
	}
}

func TestShouldOfferCompletedScroll(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()
	if err := spool.Append([]byte("one\ntwo\nthree\n")); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if shouldOfferCompletedScroll(spool, 3) {
		t.Fatal("did not expect scroll mode when output fits")
	}
	if !shouldOfferCompletedScroll(spool, 2) {
		t.Fatal("expected scroll mode when output is taller than viewport")
	}
}

func TestRenderScrollViewUsesRowsBelowStatusLine(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	writer.size = func() (rows, cols int, err error) {
		return 4, 40, nil
	}
	writer.Attach()
	base.Reset()

	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()
	if err := spool.Append([]byte("one\ntwo\nthree\nfour\n")); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	runner := newManagedOutputRunner(writer, status)
	if err := runner.renderScrollView(spool, 1); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := base.String()
	for _, want := range []string{
		"<goto:2:1>",
		"<goto:3:1>",
		"<goto:4:1>",
		"two",
		"three",
		"four",
		"SCROLL line 2/4",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected render output to contain %q, got %q", want, out)
		}
	}
}

func TestRenderLiveTailReservesPromptRow(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	writer.size = func() (rows, cols int, err error) {
		return 4, 40, nil
	}
	writer.Attach()
	base.Reset()

	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()
	if err := spool.Append([]byte("one\ntwo\nthree\nfour\n")); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()
	if err := runner.renderLiveTail(spool); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := base.String()
	for _, want := range []string{
		"<goto:2:1>",
		"<goto:3:1>",
		"three",
		"four",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected live tail output to contain %q, got %q", want, out)
		}
	}
	for _, notWant := range []string{
		"<goto:4:1><erase-line>four",
		"<goto:4:1><erase-line>three",
	} {
		if strings.Contains(out, notWant) {
			t.Fatalf("did not expect live tail to render output on prompt row, got %q", out)
		}
	}
	if !strings.Contains(out, "<goto:4:1>") {
		t.Fatalf("expected cursor to land on reserved prompt row, got %q", out)
	}
}

func TestRestoreSpoolUsesLatestOutputEntry(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	writer.size = func() (rows, cols int, err error) {
		return 4, 40, nil
	}
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	first := mustSpool(t, "first\n")
	second := mustSpool(t, "second\n")
	if _, err := runner.history.Add("first command", time.Now(), nil, first); err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	if _, err := runner.history.Add("second command", time.Now(), nil, second); err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	if got := runner.restoreSpool(first); got != second {
		t.Fatal("expected restore spool to use latest command output")
	}
}

func TestOutputEntryForSelectorUsesNewestFirstIndex(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	first := mustSpool(t, "first\n")
	second := mustSpool(t, "second\n")
	firstEntry, err := runner.history.Add("first command", time.Now(), nil, first)
	if err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	secondEntry, err := runner.history.Add("second command", time.Now(), nil, second)
	if err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	got, err := runner.outputEntryForSelector("1")
	if err != nil {
		t.Fatalf("select newest failed: %v", err)
	}
	if got != secondEntry {
		t.Fatalf("expected /outputs 1 to select newest entry, got %#v", got)
	}

	got, err = runner.outputEntryForSelector("id:" + strconvItoa(firstEntry.ID))
	if err != nil {
		t.Fatalf("select id failed: %v", err)
	}
	if got != firstEntry {
		t.Fatalf("expected id selector to select first entry, got %#v", got)
	}

	got, err = runner.outputEntryForSelector("latest")
	if err != nil {
		t.Fatalf("select latest failed: %v", err)
	}
	if got != secondEntry {
		t.Fatalf("expected latest selector to select second entry, got %#v", got)
	}
}

func TestHandlePromptCommandHelp(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	output := captureStdout(t, func() {
		if !runner.HandlePromptCommand("/help") {
			t.Fatal("expected /help to be handled")
		}
	})

	for _, expected := range []string{
		"kube-prompt cheat sheet",
		"Ctrl-S  Open output history / scroll mode when output exists",
		"/namespace [NAME]",
		"/outputs [latest|INDEX|id:ID]",
		"/output save all|last|ID PATH",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected /help output to contain %q, got %q", expected, output)
		}
	}
}

func TestManagedCommandStateHandle(t *testing.T) {
	state := &managedCommandState{}

	result, handled := state.Handle(prompt.KeyEvent{Text: "/"})
	if !handled || !result.changed || !result.refreshSuggestions || state.Input() != "/" {
		t.Fatalf("expected slash to start command mode, handled=%t result=%#v input=%q", handled, result, state.Input())
	}

	result, handled = state.Handle(prompt.KeyEvent{Text: "h"})
	if !handled || !result.changed || !result.refreshSuggestions || state.Input() != "/h" {
		t.Fatalf("expected text to append in command mode, handled=%t result=%#v input=%q", handled, result, state.Input())
	}

	result, handled = state.Handle(prompt.KeyEvent{Key: prompt.Backspace})
	if !handled || !result.changed || !result.refreshSuggestions || state.Input() != "/" {
		t.Fatalf("expected backspace to delete one rune, handled=%t result=%#v input=%q", handled, result, state.Input())
	}

	result, handled = state.Handle(prompt.KeyEvent{Key: prompt.Escape})
	if !handled || !result.cancelled || state.Active() {
		t.Fatalf("expected escape to cancel command mode, handled=%t result=%#v active=%t", handled, result, state.Active())
	}

	_, _ = state.Handle(prompt.KeyEvent{Text: "/"})
	_, _ = state.Handle(prompt.KeyEvent{Text: "h"})
	_, _ = state.Handle(prompt.KeyEvent{Text: "e"})
	_, _ = state.Handle(prompt.KeyEvent{Text: "l"})
	_, _ = state.Handle(prompt.KeyEvent{Text: "p"})
	result, handled = state.Handle(prompt.KeyEvent{Key: prompt.Enter})
	if !handled || result.submitted != "/help" || state.Active() {
		t.Fatalf("expected enter to submit command, handled=%t result=%#v active=%t", handled, result, state.Active())
	}
}

func TestManagedCommandStateConsumesNavigationWhileActive(t *testing.T) {
	state := &managedCommandState{}
	_, _ = state.Handle(prompt.KeyEvent{Text: "/"})

	result, handled := state.Handle(prompt.KeyEvent{Key: prompt.Up})
	if !handled {
		t.Fatal("expected active command mode to consume navigation keys")
	}
	if result != (managedCommandEventResult{}) {
		t.Fatalf("expected empty result for ignored navigation key, got %#v", result)
	}
	if state.Input() != "/" {
		t.Fatalf("expected command input to remain unchanged, got %q", state.Input())
	}
}

func TestManagedCommandStateAcceptsSelectedSuggestion(t *testing.T) {
	state := &managedCommandState{}
	_, _ = state.Handle(prompt.KeyEvent{Text: "/"})
	state.SetSuggestions([]prompt.Suggest{
		{Text: "/help"},
		{Text: "/outputs"},
	})

	result, handled := state.Handle(prompt.KeyEvent{Key: prompt.Tab})
	if !handled || !result.changed || result.refreshSuggestions {
		t.Fatalf("expected tab to move selection, handled=%t result=%#v", handled, result)
	}
	if suggest, ok := state.SelectedSuggestion(); !ok || suggest.Text != "/help" {
		t.Fatalf("expected first suggestion to be selected, got %#v ok=%t", suggest, ok)
	}

	result, handled = state.Handle(prompt.KeyEvent{Key: prompt.Enter})
	if !handled || !result.changed || !result.refreshSuggestions || state.Input() != "/help" {
		t.Fatalf("expected enter to accept selected suggestion, handled=%t result=%#v input=%q", handled, result, state.Input())
	}
}

func TestHandleManagedUICommand(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	first := mustSpool(t, "first\n")
	second := mustSpool(t, "second\n")
	firstEntry, err := runner.history.Add("first command", time.Now(), nil, first)
	if err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	secondEntry, err := runner.history.Add("second command", time.Now(), nil, second)
	if err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	if action := runner.handleManagedUICommand("/help", managedUIContextScroll); action.kind != managedUICommandActionShowHelp {
		t.Fatalf("expected help action, got %#v", action)
	}

	if action := runner.handleManagedUICommand("/outputs", managedUIContextScroll); action.kind != managedUICommandActionShowOutputs {
		t.Fatalf("expected show outputs action, got %#v", action)
	}

	action := runner.handleManagedUICommand("/outputs latest", managedUIContextScroll)
	if action.kind != managedUICommandActionShowEntry || action.entry != secondEntry {
		t.Fatalf("expected latest entry action, got %#v", action)
	}

	action = runner.handleManagedUICommand("/outputs id:"+strconvItoa(firstEntry.ID), managedUIContextPicker)
	if action.kind != managedUICommandActionShowEntry || action.entry != firstEntry {
		t.Fatalf("expected id entry action, got %#v", action)
	}

	action = runner.handleManagedUICommand("/outputs", managedUIContextPicker)
	if action.kind != managedUICommandActionNone || action.message != "already viewing outputs" {
		t.Fatalf("expected picker no-op message, got %#v", action)
	}

	action = runner.handleManagedUICommand("/namespace apps", managedUIContextScroll)
	if action.kind != managedUICommandActionNone || !strings.Contains(action.message, "command not available in scroll mode") {
		t.Fatalf("expected unsupported command message, got %#v", action)
	}
}

func TestHandleManagedUIOutputCommandSaveLast(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	last := mustSpool(t, "saved output\n")
	if _, err := runner.history.Add("command", time.Now(), nil, last); err != nil {
		t.Fatalf("add last failed: %v", err)
	}

	path := t.TempDir() + "/last.log"
	action := runner.handleManagedUICommand("/output save last "+path, managedUIContextScroll)
	if action.kind != managedUICommandActionNone || action.message != "saved output to "+path {
		t.Fatalf("expected save success message, got %#v", action)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file failed: %v", err)
	}
	if got := string(data); !strings.Contains(got, "saved output") {
		t.Fatalf("expected saved file contents, got %q", got)
	}
}

func TestCompleteManagedUICommand(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	runner := newManagedOutputRunner(writer, status)
	defer runner.Close()

	first := mustSpool(t, "first\n")
	second := mustSpool(t, "second\n")
	firstEntry, err := runner.history.Add("first command", time.Now(), nil, first)
	if err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	if _, err := runner.history.Add("second command", time.Now(), nil, second); err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	assertManagedSuggestionTexts(t, runner.completeManagedUICommand("/", managedUIContextScroll), []string{"/help", "/outputs", "/output"})
	assertManagedSuggestionTexts(t, runner.completeManagedUICommand("/o", managedUIContextScroll), []string{"/outputs", "/output"})
	assertManagedSuggestionContains(t, runner.completeManagedUICommand("/outputs ", managedUIContextScroll), "latest")
	assertManagedSuggestionContains(t, runner.completeManagedUICommand("/outputs ", managedUIContextScroll), "id:"+strconvItoa(firstEntry.ID))
	assertManagedSuggestionContains(t, runner.completeManagedUICommand("/output s", managedUIContextScroll), "save")
	assertManagedSuggestionContains(t, runner.completeManagedUICommand("/output save ", managedUIContextScroll), "all")
	assertManagedSuggestionContains(t, runner.completeManagedUICommand("/output save ", managedUIContextScroll), "last")
	if got := runner.completeManagedUICommand("/output save last ", managedUIContextScroll); len(got) != 0 {
		t.Fatalf("expected no suggestions for save path position, got %#v", got)
	}
}

func TestRenderScrollViewStateRendersManagedCommandDropdown(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	writer.size = func() (rows, cols int, err error) {
		return 7, 80, nil
	}
	writer.Attach()
	base.Reset()

	spool := mustSpool(t, "one\ntwo\nthree\nfour\n")
	defer spool.Close()

	command := &managedCommandState{input: []rune("/o")}
	command.SetSuggestions([]prompt.Suggest{
		{Text: "/output", Description: "Save captured output to a file"},
		{Text: "/outputs", Description: "Browse captured command output"},
	})

	runner := newManagedOutputRunner(writer, status)
	if err := runner.renderScrollViewState(spool, 0, command); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := base.String()
	if strings.Contains(out, "COMMAND /o") {
		t.Fatalf("did not expect command text in status line, got %q", out)
	}
	for _, want := range []string{
		"SCROLL line 1/4",
		"<goto:2:1><erase-line>one",
		"<goto:4:1><erase-line>three",
		"/output",
		"/outputs",
		"<goto:5:1>",
		"<goto:6:1>",
		"<goto:7:1><erase-line>/o",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected dropdown render output to contain %q, got %q", want, out)
		}
	}
}

func TestRenderLiveTailStateRendersManagedCommandAtBottom(t *testing.T) {
	base := &recordingPromptWriter{}
	status := &outputModeStatus{}
	writer := newDynamicStatusLineWriter(base, func() string {
		return " status" + status.Suffix()
	})
	writer.size = func() (rows, cols int, err error) {
		return 6, 80, nil
	}
	writer.Attach()
	base.Reset()

	spool := mustSpool(t, "one\ntwo\nthree\nfour\n")
	defer spool.Close()

	command := &managedCommandState{input: []rune("/o")}
	command.SetSuggestions([]prompt.Suggest{
		{Text: "/output", Description: "Save captured output to a file"},
		{Text: "/outputs", Description: "Browse captured command output"},
	})

	runner := newManagedOutputRunner(writer, status)
	if err := runner.renderLiveTailState(spool, command); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	out := base.String()
	if strings.Contains(out, "COMMAND /o") {
		t.Fatalf("did not expect command text in status line, got %q", out)
	}
	for _, want := range []string{
		"<goto:2:1><erase-line>three",
		"<goto:3:1><erase-line>four",
		"<goto:4:1>",
		"<goto:5:1>",
		"<goto:6:1><erase-line>/o",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected live tail render output to contain %q, got %q", want, out)
		}
	}
}

func assertManagedSuggestionTexts(t *testing.T, suggestions []prompt.Suggest, expected []string) {
	t.Helper()

	if len(suggestions) != len(expected) {
		t.Fatalf("expected %d suggestions, got %#v", len(expected), suggestions)
	}
	for i, want := range expected {
		if suggestions[i].Text != want {
			t.Fatalf("expected suggestion %d to be %q, got %#v", i, want, suggestions)
		}
	}
}

func assertManagedSuggestionContains(t *testing.T, suggestions []prompt.Suggest, want string) {
	t.Helper()

	for _, suggestion := range suggestions {
		if suggestion.Text == want {
			return
		}
	}
	t.Fatalf("expected suggestion %q in %#v", want, suggestions)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader failed: %v", err)
	}
	return string(out)
}
