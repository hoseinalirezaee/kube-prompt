package prompt

import (
	"reflect"
	"testing"
	"time"
)

func TestSplitInputAtEnter(t *testing.T) {
	prefix, rest, found := splitInputAtEnter([]byte("get nodes\nget pods\n"))
	if !found {
		t.Fatal("expected newline-delimited input to split")
	}
	if got, want := string(prefix), "get nodes\n"; got != want {
		t.Fatalf("expected prefix %q, got %q", want, got)
	}
	if got, want := string(rest), "get pods\n"; got != want {
		t.Fatalf("expected rest %q, got %q", want, got)
	}

	prefix, rest, found = splitInputAtEnter([]byte("get pods"))
	if found {
		t.Fatal("did not expect split without enter")
	}
	if prefix != nil {
		t.Fatalf("expected nil prefix, got %q", string(prefix))
	}
	if got, want := string(rest), "get pods"; got != want {
		t.Fatalf("expected rest %q, got %q", want, got)
	}
}

func TestSplitInputAtEnterKeepsCRLFTogether(t *testing.T) {
	prefix, rest, found := splitInputAtEnter([]byte("get nodes\r\nget pods\r\n"))
	if !found {
		t.Fatal("expected CRLF-delimited input to split")
	}
	if got, want := string(prefix), "get nodes\r\n"; got != want {
		t.Fatalf("expected prefix %q, got %q", want, got)
	}
	if got, want := string(rest), "get pods\r\n"; got != want {
		t.Fatalf("expected rest %q, got %q", want, got)
	}
}

func TestRunDoesNotBlockInputOnSlowCompleter(t *testing.T) {
	parser := &scriptedParser{input: make(chan []byte, 10)}
	completerStarted := make(chan struct{}, 1)
	releaseCompleter := make(chan struct{})
	executed := make(chan string, 1)

	p := newTestPrompt(
		parser,
		discardWriter{},
		func(input string) {
			executed <- input
		},
		func(Document) []Suggest {
			select {
			case completerStarted <- struct{}{}:
			default:
			}
			<-releaseCompleter
			return []Suggest{{Text: "slow"}}
		},
	)
	p.exitChecker = func(_ string, breakline bool) bool {
		return breakline
	}

	done := make(chan struct{})
	go func() {
		p.Run()
		close(done)
	}()

	parser.input <- []byte("a")
	select {
	case <-completerStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected completer to start")
	}

	parser.input <- []byte("b")
	parser.input <- []byte("\n")

	select {
	case got := <-executed:
		if got != "ab" {
			t.Fatalf("expected executor input %q, got %q", "ab", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("prompt input blocked on slow completer")
	}

	close(releaseCompleter)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prompt did not exit")
	}
}

func TestCompletionStateDiscardsStaleResults(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	state := newCompletionState(func(doc Document) []Suggest {
		if doc.Text == "old" {
			firstStarted <- struct{}{}
			<-releaseFirst
		}
		return []Suggest{{Text: doc.Text}}
	})
	defer state.Stop()

	manager := NewCompletionManager(func(Document) []Suggest { return nil }, 6)
	state.Request(Document{Text: "old", cursorPosition: len("old")})

	select {
	case <-firstStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected first completion to start")
	}

	state.Request(Document{Text: "new", cursorPosition: len("new")})
	close(releaseFirst)

	deadline := time.After(time.Second)
	for {
		select {
		case result := <-state.Results():
			if !state.Apply(manager, result) {
				continue
			}
			if got, want := manager.GetSuggestions(), []Suggest{{Text: "new"}}; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected latest suggestions %#v, got %#v", want, got)
			}
			return
		case <-deadline:
			t.Fatal("expected latest completion result")
		}
	}
}

func TestShouldRequestCompletionForTabWithoutSuggestions(t *testing.T) {
	p := newTestPrompt(
		&scriptedParser{input: make(chan []byte, 1)},
		discardWriter{},
		func(string) {},
		func(Document) []Suggest { return nil },
	)
	p.buf.InsertText("get p", false, true)
	before := p.currentDocumentState()

	p.buf.lastKeyStroke = Tab
	if !p.shouldRequestCompletion(before) {
		t.Fatal("expected tab to request completion when no suggestions are loaded")
	}

	p.completion.SetSuggestions([]Suggest{{Text: "pods"}})
	if p.shouldRequestCompletion(before) {
		t.Fatal("did not expect tab navigation to refresh loaded suggestions")
	}
}

func TestCompletionRefreshPreservesSuggestionsWhilePending(t *testing.T) {
	p := newTestPrompt(
		&scriptedParser{input: make(chan []byte, 1)},
		discardWriter{},
		func(string) {},
		func(Document) []Suggest { return nil },
	)
	p.keyParser = NewKeyParser()
	p.buf.InsertText("p", false, true)
	p.completion.SetSuggestions([]Suggest{
		{Text: "pods"},
		{Text: "proxy"},
	})

	before := p.currentDocumentState()
	previous := p.currentSuggestions()
	shouldExit, exec := p.feed([]byte("o"))
	if shouldExit || exec != nil {
		t.Fatalf("expected text input only, shouldExit=%t exec=%#v", shouldExit, exec)
	}
	if !p.shouldRequestCompletion(before) {
		t.Fatal("expected typing to request refreshed completions")
	}

	p.prepareCompletionRefresh(previous)
	if got, want := p.completion.GetSuggestions(), previous; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected previous suggestions to stay visible, want %#v, got %#v", want, got)
	}
	if p.completion.Completing() {
		t.Fatal("expected selection preview to reset while preserving suggestions")
	}
}

func TestCompletionRefreshHardResetKeysCloseSuggestions(t *testing.T) {
	p := newTestPrompt(
		&scriptedParser{input: make(chan []byte, 1)},
		discardWriter{},
		func(string) {},
		func(Document) []Suggest { return nil },
	)
	p.completion.SetSuggestions([]Suggest{{Text: "pods"}})
	previous := p.currentSuggestions()
	p.buf.lastKeyStroke = Escape

	p.prepareCompletionRefresh(previous)
	if got := p.completion.GetSuggestions(); len(got) != 0 {
		t.Fatalf("expected hard reset key to clear suggestions, got %#v", got)
	}
}

type scriptedParser struct {
	input chan []byte
}

func (p *scriptedParser) Setup() error {
	return nil
}

func (p *scriptedParser) TearDown() error {
	return nil
}

func (p *scriptedParser) GetWinSize() *WinSize {
	return &WinSize{Row: 24, Col: 80}
}

func (p *scriptedParser) Read() ([]byte, error) {
	select {
	case b := <-p.input:
		return b, nil
	default:
		return []byte{0}, nil
	}
}

type discardWriter struct{}

func newTestPrompt(parser ConsoleParser, writer ConsoleWriter, executor Executor, completer Completer) *Prompt {
	registerConsoleWriter(writer)
	return &Prompt{
		in: parser,
		renderer: &Render{
			prefix:                       "> ",
			out:                          writer,
			livePrefixCallback:           func() (string, bool) { return "", false },
			prefixTextColor:              Blue,
			prefixBGColor:                DefaultColor,
			inputTextColor:               DefaultColor,
			inputBGColor:                 DefaultColor,
			previewSuggestionTextColor:   Green,
			previewSuggestionBGColor:     DefaultColor,
			suggestionTextColor:          White,
			suggestionBGColor:            Cyan,
			selectedSuggestionTextColor:  Black,
			selectedSuggestionBGColor:    Turquoise,
			descriptionTextColor:         Black,
			descriptionBGColor:           Turquoise,
			selectedDescriptionTextColor: White,
			selectedDescriptionBGColor:   Cyan,
			scrollbarThumbColor:          DarkGray,
			scrollbarBGColor:             Cyan,
		},
		buf:         NewBuffer(),
		executor:    executor,
		history:     NewHistory(),
		completion:  NewCompletionManager(completer, 6),
		keyBindMode: EmacsKeyBind,
	}
}

func (discardWriter) WriteRaw([]byte)             {}
func (discardWriter) Write([]byte)                {}
func (discardWriter) WriteRawStr(string)          {}
func (discardWriter) WriteStr(string)             {}
func (discardWriter) Flush() error                { return nil }
func (discardWriter) EraseScreen()                {}
func (discardWriter) EraseUp()                    {}
func (discardWriter) EraseDown()                  {}
func (discardWriter) EraseStartOfLine()           {}
func (discardWriter) EraseEndOfLine()             {}
func (discardWriter) EraseLine()                  {}
func (discardWriter) ShowCursor()                 {}
func (discardWriter) HideCursor()                 {}
func (discardWriter) CursorGoTo(int, int)         {}
func (discardWriter) CursorUp(int)                {}
func (discardWriter) CursorDown(int)              {}
func (discardWriter) CursorForward(int)           {}
func (discardWriter) CursorBackward(int)          {}
func (discardWriter) AskForCPR()                  {}
func (discardWriter) SaveCursor()                 {}
func (discardWriter) UnSaveCursor()               {}
func (discardWriter) ScrollDown()                 {}
func (discardWriter) ScrollUp()                   {}
func (discardWriter) SetTitle(string)             {}
func (discardWriter) ClearTitle()                 {}
func (discardWriter) SetColor(Color, Color, bool) {}
