package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	runewidth "github.com/mattn/go-runewidth"
)

type outputModeStatus struct {
	mu      sync.Mutex
	active  bool
	line    int
	total   int
	message string
}

func (s *outputModeStatus) SetScroll(line, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.line = line
	s.total = total
	s.message = ""
}

func (s *outputModeStatus) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	s.line = 0
	s.total = 0
	s.message = ""
}

func (s *outputModeStatus) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.line = 0
	s.total = 0
	s.message = message
}

func (s *outputModeStatus) Suffix() string {
	label := s.Label()
	if label == "" {
		return ""
	}
	return " | " + label
}

func (s *outputModeStatus) Label() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return ""
	}
	if s.message != "" {
		return s.message
	}
	if s.total == 0 {
		return "SCROLL line 0/0"
	}
	return "SCROLL line " + strconvItoa(s.line) + "/" + strconvItoa(s.total)
}

type managedOutputRunner struct {
	statusWriter *statusLineWriter
	modeStatus   *outputModeStatus
	history      *outputHistory
}

type managedUIContext string

const (
	managedUIContextScroll managedUIContext = "scroll"
	managedUIContextPicker managedUIContext = "picker"
)

type managedCommandState struct {
	input          []rune
	suggestions    []prompt.Suggest
	selected       int
	verticalScroll int
}

type managedCommandEventResult struct {
	cancelled          bool
	changed            bool
	refreshSuggestions bool
	submitted          string
}

const managedCommandMaxSuggestions = 6

func (s *managedCommandState) Active() bool {
	return len(s.input) > 0
}

func (s *managedCommandState) Input() string {
	return string(s.input)
}

func (s *managedCommandState) Clear() {
	s.input = nil
	s.suggestions = nil
	s.selected = -1
	s.verticalScroll = 0
}

func (s *managedCommandState) SetSuggestions(suggestions []prompt.Suggest) {
	s.suggestions = suggestions
	s.selected = -1
	s.verticalScroll = 0
}

func (s *managedCommandState) Suggestions() []prompt.Suggest {
	return s.suggestions
}

func (s *managedCommandState) HasSelectedSuggestion() bool {
	return s.selected >= 0 && s.selected < len(s.suggestions)
}

func (s *managedCommandState) SelectedSuggestion() (prompt.Suggest, bool) {
	if !s.HasSelectedSuggestion() {
		return prompt.Suggest{}, false
	}
	return s.suggestions[s.selected], true
}

func (s *managedCommandState) DropdownHeight() int {
	if len(s.suggestions) == 0 {
		return 0
	}
	return minInt(len(s.suggestions), managedCommandMaxSuggestions)
}

func (s *managedCommandState) VisibleSuggestions() []prompt.Suggest {
	if len(s.suggestions) == 0 {
		return nil
	}
	end := minInt(len(s.suggestions), s.verticalScroll+s.DropdownHeight())
	return s.suggestions[s.verticalScroll:end]
}

func (s *managedCommandState) selectNext() bool {
	if len(s.suggestions) == 0 {
		return false
	}
	if s.verticalScroll+managedCommandMaxSuggestions-1 == s.selected {
		s.verticalScroll++
	}
	s.selected++
	s.updateSelection()
	return true
}

func (s *managedCommandState) selectPrevious() bool {
	if len(s.suggestions) == 0 {
		return false
	}
	if s.verticalScroll == s.selected && s.selected > 0 {
		s.verticalScroll--
	}
	s.selected--
	s.updateSelection()
	return true
}

func (s *managedCommandState) updateSelection() {
	maxVisible := managedCommandMaxSuggestions
	if len(s.suggestions) < maxVisible {
		maxVisible = len(s.suggestions)
	}
	if s.selected >= len(s.suggestions) {
		s.selected = -1
		s.verticalScroll = 0
	} else if s.selected < -1 {
		s.selected = len(s.suggestions) - 1
		s.verticalScroll = maxInt(0, len(s.suggestions)-maxVisible)
	}
}

func (s *managedCommandState) acceptSelectedSuggestion() bool {
	suggest, ok := s.SelectedSuggestion()
	if !ok {
		return false
	}

	input := s.Input()
	if input == "" {
		s.input = []rune(suggest.Text)
		return true
	}
	if endsWithWhitespace(input) {
		s.input = []rune(input + suggest.Text)
		return true
	}

	lastSpace := strings.LastIndexFunc(input, unicode.IsSpace)
	if lastSpace == -1 {
		s.input = []rune(suggest.Text)
		return true
	}
	s.input = []rune(input[:lastSpace+1] + suggest.Text)
	return true
}

func (s *managedCommandState) Handle(event prompt.KeyEvent) (managedCommandEventResult, bool) {
	var result managedCommandEventResult

	if !s.Active() {
		if event.Text == "/" {
			s.input = []rune{'/'}
			result.changed = true
			result.refreshSuggestions = true
			return result, true
		}
		return result, false
	}

	if event.Text != "" {
		s.input = append(s.input, []rune(event.Text)...)
		s.selected = -1
		s.verticalScroll = 0
		result.changed = true
		result.refreshSuggestions = true
		return result, true
	}

	switch event.Key {
	case prompt.Tab, prompt.ControlI, prompt.Down:
		if s.selectNext() {
			result.changed = true
		}
		return result, true
	case prompt.BackTab, prompt.Up:
		if s.selectPrevious() {
			result.changed = true
		}
		return result, true
	case prompt.Enter, prompt.ControlM, prompt.ControlJ:
		if s.acceptSelectedSuggestion() {
			s.selected = -1
			s.verticalScroll = 0
			result.changed = true
			result.refreshSuggestions = true
			return result, true
		}
		result.submitted = s.Input()
		s.Clear()
		return result, true
	case prompt.Escape:
		s.Clear()
		result.cancelled = true
		return result, true
	case prompt.Backspace, prompt.ControlH:
		if len(s.input) > 0 {
			s.input = s.input[:len(s.input)-1]
		}
		s.selected = -1
		s.verticalScroll = 0
		if len(s.input) == 0 {
			result.cancelled = true
		} else {
			result.changed = true
			result.refreshSuggestions = true
		}
		return result, true
	}

	// Ignore non-text navigation keys while command entry is active.
	return result, true
}

type managedUICommandActionKind int

const (
	managedUICommandActionNone managedUICommandActionKind = iota
	managedUICommandActionShowHelp
	managedUICommandActionShowOutputs
	managedUICommandActionShowEntry
)

type managedUICommandAction struct {
	kind    managedUICommandActionKind
	entry   *outputEntry
	message string
}

func newManagedOutputRunner(statusWriter *statusLineWriter, modeStatus *outputModeStatus) *managedOutputRunner {
	history, err := newOutputHistory()
	if err != nil {
		history = nil
	}
	return &managedOutputRunner{
		statusWriter: statusWriter,
		modeStatus:   modeStatus,
		history:      history,
	}
}

func (r *managedOutputRunner) Run(input string, cmd *exec.Cmd) error {
	if r.history == nil || !shouldManageCommand(input) || !r.statusWriter.attached {
		return runDirectCommand(input, cmd)
	}
	if !isStreamingCommand(input) {
		return r.runCaptured(input, cmd)
	}
	if err := r.runManaged(input, cmd); err != errManagedUnavailable {
		return err
	}
	return runDirectCommand(input, cmd)
}

func (r *managedOutputRunner) Close() {
	if r.history != nil {
		_ = r.history.Close()
	}
}

var errManagedUnavailable = errors.New("managed output unavailable")

type outputChunk struct {
	data []byte
}

func (r *managedOutputRunner) runCaptured(input string, cmd *exec.Cmd) error {
	spool, err := newOutputSpool()
	if err != nil {
		return err
	}
	defer r.modeStatus.Clear()
	defer r.statusWriter.Flush()
	startedAt := time.Now()

	cmd.Stdin = nil
	cmd.Stdout = spool
	cmd.Stderr = spool
	waitErr := cmd.Run()

	if err := r.renderCompletedOutput(spool); err != nil {
		_ = spool.Close()
		return err
	}
	if err := normalizePromptCursor(spool); err != nil {
		_ = spool.Close()
		return err
	}
	if _, err := r.history.Add(input, startedAt, waitErr, spool); err != nil {
		_ = spool.Close()
		return err
	}
	return waitErr
}

func (r *managedOutputRunner) runManaged(input string, cmd *exec.Cmd) error {
	spool, err := newOutputSpool()
	if err != nil {
		return err
	}
	defer r.modeStatus.Clear()
	defer r.statusWriter.Flush()
	startedAt := time.Now()

	tty, err := openManagedTTY()
	if err != nil {
		_ = spool.Close()
		return errManagedUnavailable
	}
	ttyClosed := false
	closeTTY := func() {
		if ttyClosed {
			return
		}
		ttyClosed = true
		_ = tty.Close()
	}
	defer closeTTY()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdin = nil
	prepareManagedCommand(cmd)

	if err := cmd.Start(); err != nil {
		_ = spool.Close()
		return err
	}

	chunks := make(chan outputChunk, 64)
	var copyWG sync.WaitGroup
	copyPipe := func(r io.Reader) {
		defer copyWG.Done()
		buf := make([]byte, 8192)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				chunks <- outputChunk{data: data}
			}
			if err != nil {
				return
			}
		}
	}
	copyWG.Add(2)
	go copyPipe(stdout)
	go copyPipe(stderr)
	go func() {
		copyWG.Wait()
		close(chunks)
	}()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	keyParser := prompt.NewKeyParser()
	command := &managedCommandState{}
	scrolling := false
	interrupted := false
	topLine := 0
	waitErr := error(nil)
	processDone := false
	chunksDone := false
	liveOutput := &terminalOutputWriter{w: os.Stdout}

	handleKeyEvent := func(event prompt.KeyEvent) error {
		if scrolling {
			if result, handled := command.Handle(event); handled {
				switch {
				case result.cancelled:
					if err := r.renderScrollViewState(spool, topLine, command); err != nil {
						return err
					}
				case result.changed:
					if result.refreshSuggestions {
						r.updateManagedCommandSuggestions(command, managedUIContextScroll)
					}
					if err := r.renderScrollViewState(spool, topLine, command); err != nil {
						return err
					}
				case result.submitted != "":
					action := r.handleManagedUICommand(result.submitted, managedUIContextScroll)
					switch action.kind {
					case managedUICommandActionShowHelp:
						if err := r.showHelpView(); err != nil {
							return err
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionShowOutputs:
						if err := r.showOutputPicker(); err != nil {
							return err
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionShowEntry:
						if action.entry != nil {
							if err := r.showScrollView(action.entry.Spool, maxInt(0, action.entry.Spool.LineCount()-r.visibleRows())); err != nil {
								return err
							}
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionNone:
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
						if action.message != "" {
							r.modeStatus.SetMessage(action.message)
							_ = r.statusWriter.Flush()
						}
					}
				}
				return nil
			}
		}

		switch {
		case event.Key == prompt.ControlC:
			interrupted = true
			if cmd.Process != nil && !processDone {
				_ = interruptManagedCommand(cmd)
			}
			if scrolling {
				scrolling = false
				r.modeStatus.Clear()
				_ = r.statusWriter.Flush()
			}
		case event.Key == prompt.ControlS:
			if scrolling {
				scrolling = false
				r.modeStatus.Clear()
				if err := r.renderLiveTailState(spool, command); err != nil {
					return err
				}
				_ = r.statusWriter.Flush()
				return nil
			}
			scrolling = true
			topLine = maxInt(0, spool.LineCount()-r.visibleRows())
			if err := r.renderScrollView(spool, topLine); err != nil {
				return err
			}
		case scrolling && (event.Key == prompt.Escape || event.Text == "q"):
			scrolling = false
			r.modeStatus.Clear()
			if err := r.renderLiveTail(spool); err != nil {
				return err
			}
			_ = r.statusWriter.Flush()
		case scrolling && event.Text == "w":
			path := defaultOutputSavePath()
			if err := saveSpool(spool, path); err == nil {
				r.modeStatus.SetMessage("SAVED " + path)
				_ = r.statusWriter.Flush()
			}
		case scrolling:
			newTop := handleScrollKey(event, topLine, spool.LineCount(), r.visibleRows())
			if newTop != topLine {
				topLine = newTop
				if err := r.renderScrollView(spool, topLine); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for {
		for _, event := range readManagedKeyEvents(tty, keyParser) {
			if err := handleKeyEvent(event); err != nil {
				return err
			}
		}

		if processDone && chunksDone && !scrolling {
			if err := normalizePromptCursor(spool); err != nil {
				_ = spool.Close()
				return err
			}
			if _, err := r.history.Add(input, startedAt, waitErr, spool); err != nil {
				_ = spool.Close()
				return err
			}
			if interrupted {
				return nil
			}
			return waitErr
		}

		select {
		case chunk, ok := <-chunks:
			if !ok {
				chunksDone = true
				chunks = nil
				continue
			}
			if err := spool.Append(chunk.data); err != nil {
				return err
			}
			if interrupted {
				continue
			}
			if scrolling {
				if command.Active() {
					continue
				}
				r.updateScrollStatus(topLine, spool)
				_ = r.statusWriter.Flush()
				continue
			}
			if _, err := liveOutput.Write(chunk.data); err != nil {
				return err
			}
		case err := <-waitCh:
			processDone = true
			waitErr = err
			waitCh = nil
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (r *managedOutputRunner) renderCompletedOutput(spool *outputSpool) error {
	if shouldOfferCompletedScroll(spool, r.liveTailRows()) {
		return r.renderLiveTail(spool)
	}
	return spool.WriteTo(os.Stdout)
}

func (r *managedOutputRunner) ShowHistory() {
	if r.history == nil {
		return
	}
	spool := r.history.Continuous()
	if spool == nil || spool.LineCount() == 0 {
		return
	}
	_ = r.showScrollView(spool, maxInt(0, spool.LineCount()-r.visibleRows()))
}

func (r *managedOutputRunner) ShowOutputPicker() {
	if r.history == nil {
		return
	}
	_ = r.showOutputPicker()
}

func (r *managedOutputRunner) HandlePromptCommand(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}
	fields := strings.Fields(input)
	switch fields[0] {
	case "/help":
		fmt.Fprint(os.Stdout, cheatSheetText())
		return true
	case "/outputs":
		if len(fields) == 1 {
			r.ShowOutputPicker()
			return true
		}
		if len(fields) == 2 {
			entry, err := r.outputEntryForSelector(fields[1])
			if err != nil {
				fmt.Fprintln(os.Stdout, err)
				return true
			}
			r.ShowOutputEntry(entry)
			return true
		}
		fmt.Fprintln(os.Stdout, "usage: /outputs [latest|INDEX|id:ID]")
		return true
	case "/output":
		r.handleOutputCommand(fields)
		return true
	default:
		return false
	}
}

func (r *managedOutputRunner) updateManagedCommandSuggestions(command *managedCommandState, ctx managedUIContext) {
	if command == nil || !command.Active() {
		return
	}
	command.SetSuggestions(r.completeManagedUICommand(command.Input(), ctx))
}

func (r *managedOutputRunner) completeManagedUICommand(input string, _ managedUIContext) []prompt.Suggest {
	input = strings.TrimLeftFunc(input, unicode.IsSpace)
	if input == "" {
		return nil
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}
	endsWithSpace := endsWithWhitespace(input)

	switch fields[0] {
	case "/help":
		if len(fields) > 1 || endsWithSpace {
			return nil
		}
	case "/outputs":
		if len(fields) == 1 && !endsWithSpace {
			return filterManagedSuggestions(managedCommandSuggestions(), fields[0])
		}
		if len(fields) == 1 && endsWithSpace {
			return r.outputSelectorSuggestions("")
		}
		if len(fields) == 2 && !endsWithSpace {
			return r.outputSelectorSuggestions(fields[1])
		}
		return nil
	case "/output":
		if len(fields) == 1 && !endsWithSpace {
			return filterManagedSuggestions(managedCommandSuggestions(), fields[0])
		}
		if len(fields) == 1 && endsWithSpace {
			return filterManagedSuggestions([]prompt.Suggest{
				{Text: "save", Description: "Save captured output to a file"},
			}, "")
		}
		if len(fields) == 2 && !endsWithSpace {
			return filterManagedSuggestions([]prompt.Suggest{
				{Text: "save", Description: "Save captured output to a file"},
			}, fields[1])
		}
		if len(fields) == 2 && endsWithSpace {
			if fields[1] != "save" {
				return nil
			}
			return r.outputSaveTargetSuggestions("")
		}
		if len(fields) == 3 && fields[1] == "save" && !endsWithSpace {
			return r.outputSaveTargetSuggestions(fields[2])
		}
		return nil
	default:
		if len(fields) == 1 {
			return filterManagedSuggestions(managedCommandSuggestions(), fields[0])
		}
	}

	return nil
}

func managedCommandSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "/help", Description: "Show the kube-prompt cheat sheet"},
		{Text: "/outputs", Description: "Browse captured command output"},
		{Text: "/output", Description: "Save captured output to a file"},
	}
}

func (r *managedOutputRunner) outputSelectorSuggestions(filter string) []prompt.Suggest {
	suggestions := []prompt.Suggest{
		{Text: "latest", Description: "Open the latest captured output"},
		{Text: "last", Description: "Open the latest captured output"},
	}
	for _, summary := range r.outputSummaries() {
		description := formatOutputSummary(summary)
		suggestions = append(suggestions,
			prompt.Suggest{Text: strconvItoa(summary.Index), Description: description},
			prompt.Suggest{Text: "id:" + strconvItoa(summary.ID), Description: description},
		)
	}
	return filterManagedSuggestions(suggestions, filter)
}

func (r *managedOutputRunner) outputSaveTargetSuggestions(filter string) []prompt.Suggest {
	suggestions := []prompt.Suggest{
		{Text: "all", Description: "Save the continuous output history"},
		{Text: "last", Description: "Save the latest captured output"},
	}
	for _, summary := range r.outputSummaries() {
		description := formatOutputSummary(summary)
		suggestions = append(suggestions,
			prompt.Suggest{Text: strconvItoa(summary.Index), Description: description},
			prompt.Suggest{Text: "id:" + strconvItoa(summary.ID), Description: description},
		)
	}
	return filterManagedSuggestions(suggestions, filter)
}

func (r *managedOutputRunner) outputSummaries() []outputEntrySummary {
	if r.history == nil {
		return nil
	}
	return r.history.SummariesNewestFirst()
}

func filterManagedSuggestions(suggestions []prompt.Suggest, prefix string) []prompt.Suggest {
	return prompt.FilterHasPrefix(suggestions, prefix, true)
}

func (r *managedOutputRunner) handleManagedUICommand(input string, ctx managedUIContext) managedUICommandAction {
	input = strings.TrimSpace(input)
	if input == "" {
		return managedUICommandAction{}
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return managedUICommandAction{}
	}

	switch fields[0] {
	case "/help":
		return managedUICommandAction{kind: managedUICommandActionShowHelp}
	case "/outputs":
		if len(fields) == 1 {
			if ctx == managedUIContextPicker {
				return managedUICommandAction{message: "already viewing outputs"}
			}
			return managedUICommandAction{kind: managedUICommandActionShowOutputs}
		}
		if len(fields) == 2 {
			entry, err := r.outputEntryForSelector(fields[1])
			if err != nil {
				return managedUICommandAction{message: err.Error()}
			}
			return managedUICommandAction{kind: managedUICommandActionShowEntry, entry: entry}
		}
		return managedUICommandAction{message: "usage: /outputs [latest|INDEX|id:ID]"}
	case "/output":
		return r.handleManagedUIOutputCommand(fields)
	default:
		return managedUICommandAction{message: "command not available in scroll mode: " + fields[0]}
	}
}

func (r *managedOutputRunner) handleManagedUIOutputCommand(fields []string) managedUICommandAction {
	if r.history == nil || len(fields) < 2 || fields[1] != "save" {
		return managedUICommandAction{message: "usage: /output save all|last|ID PATH"}
	}
	if len(fields) < 4 {
		return managedUICommandAction{message: "usage: /output save all|last|ID PATH"}
	}

	target := fields[2]
	path := strings.Join(fields[3:], " ")
	var err error
	switch target {
	case "all":
		err = r.history.SaveAll(path)
	case "last":
		err = r.history.SaveLatest(path)
	default:
		var id int
		id, err = parseOutputID(target)
		if err == nil {
			err = r.history.SaveID(id, path)
		}
	}
	if err != nil {
		return managedUICommandAction{message: "failed to save output: " + err.Error()}
	}
	return managedUICommandAction{message: "saved output to " + path}
}

func (r *managedOutputRunner) handleOutputCommand(fields []string) {
	if r.history == nil || len(fields) < 2 || fields[1] != "save" {
		printOutputCommandUsage(os.Stdout)
		return
	}
	if len(fields) < 4 {
		printOutputCommandUsage(os.Stdout)
		return
	}
	target := fields[2]
	path := strings.Join(fields[3:], " ")
	var err error
	switch target {
	case "all":
		err = r.history.SaveAll(path)
	case "last":
		err = r.history.SaveLatest(path)
	default:
		var id int
		id, err = parseOutputID(target)
		if err == nil {
			err = r.history.SaveID(id, path)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to save output: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stdout, "saved output to %s\n", path)
}

func (r *managedOutputRunner) outputEntryForSelector(selector string) (*outputEntry, error) {
	if r.history == nil {
		return nil, errors.New("no output history")
	}
	switch strings.ToLower(selector) {
	case "latest", "last":
		entry := r.history.Latest()
		if entry == nil {
			return nil, errors.New("no output history")
		}
		return entry, nil
	}
	if strings.HasPrefix(strings.ToLower(selector), "id:") {
		id, err := parseOutputID(selector[len("id:"):])
		if err != nil {
			return nil, err
		}
		entry := r.history.Get(id)
		if entry == nil {
			return nil, fmt.Errorf("output id:%d not found", id)
		}
		return entry, nil
	}

	index, err := parseOutputID(selector)
	if err != nil {
		return nil, err
	}
	entry := r.history.GetNewest(index)
	if entry == nil {
		return nil, fmt.Errorf("output %d not found", index)
	}
	return entry, nil
}

func (r *managedOutputRunner) ShowOutput(id int) {
	if r.history == nil {
		return
	}
	entry := r.history.Get(id)
	if entry == nil {
		fmt.Fprintf(os.Stdout, "output id:%d not found\n", id)
		return
	}
	r.ShowOutputEntry(entry)
}

func (r *managedOutputRunner) ShowOutputEntry(entry *outputEntry) {
	if entry == nil {
		return
	}
	_ = r.showScrollView(entry.Spool, maxInt(0, entry.Spool.LineCount()-r.visibleRows()))
}

func (r *managedOutputRunner) showScrollView(spool *outputSpool, topLine int) error {
	tty, err := openManagedTTY()
	if err != nil {
		return nil
	}
	defer tty.Close()

	keyParser := prompt.NewKeyParser()
	command := &managedCommandState{}
	if err := r.renderScrollViewState(spool, topLine, command); err != nil {
		return err
	}

	for {
		for _, event := range readManagedKeyEvents(tty, keyParser) {
			if result, handled := command.Handle(event); handled {
				switch {
				case result.cancelled:
					if err := r.renderScrollViewState(spool, topLine, command); err != nil {
						return err
					}
				case result.changed:
					if result.refreshSuggestions {
						r.updateManagedCommandSuggestions(command, managedUIContextScroll)
					}
					if err := r.renderScrollViewState(spool, topLine, command); err != nil {
						return err
					}
				case result.submitted != "":
					action := r.handleManagedUICommand(result.submitted, managedUIContextScroll)
					switch action.kind {
					case managedUICommandActionShowHelp:
						if err := r.showHelpView(); err != nil {
							return err
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionShowOutputs:
						if err := r.showOutputPicker(); err != nil {
							return err
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionShowEntry:
						if action.entry != nil {
							if err := r.showScrollView(action.entry.Spool, maxInt(0, action.entry.Spool.LineCount()-r.visibleRows())); err != nil {
								return err
							}
						}
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
					case managedUICommandActionNone:
						if err := r.renderScrollViewState(spool, topLine, command); err != nil {
							return err
						}
						if action.message != "" {
							r.modeStatus.SetMessage(action.message)
							_ = r.statusWriter.Flush()
						}
					}
				}
				continue
			}

			switch {
			case event.Key == prompt.ControlS || event.Key == prompt.Escape || event.Text == "q" || event.Key == prompt.ControlC:
				r.modeStatus.Clear()
				if err := r.renderLiveTail(r.restoreSpool(spool)); err != nil {
					return err
				}
				_ = r.statusWriter.Flush()
				return nil
			case event.Text == "w":
				path := defaultOutputSavePath()
				if err := saveSpool(spool, path); err == nil {
					r.modeStatus.SetMessage("SAVED " + path)
					_ = r.statusWriter.Flush()
				}
			default:
				newTop := handleScrollKey(event, topLine, spool.LineCount(), r.visibleRows())
				if newTop != topLine {
					topLine = newTop
					if err := r.renderScrollViewState(spool, topLine, command); err != nil {
						return err
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *managedOutputRunner) showOutputPicker() error {
	summaries := r.history.SummariesNewestFirst()
	if len(summaries) == 0 {
		_, _ = os.Stdout.Write([]byte("No output history\r\n"))
		return nil
	}

	tty, err := openManagedTTY()
	if err != nil {
		return nil
	}
	defer tty.Close()

	keyParser := prompt.NewKeyParser()
	command := &managedCommandState{}
	selected := 0
	top := 0
	render := func() error {
		if !r.statusWriter.refreshSize() {
			return nil
		}
		bodyRows := r.visibleRows()
		commandRow := 0
		if command.Active() {
			commandRow = r.statusWriter.rows
			bodyRows = maxInt(0, bodyRows-1)
		}
		dropdownHeight := r.dropdownHeight(command, bodyRows)
		contentRows := maxInt(0, bodyRows-dropdownHeight)
		dropdownRow := 2 + contentRows
		if selected < top {
			top = selected
		}
		if contentRows > 0 && selected >= top+contentRows {
			top = selected - contentRows + 1
		}
		r.modeStatus.SetMessage("OUTPUTS " + strconvItoa(selected+1) + "/" + strconvItoa(len(summaries)))

		w := r.statusWriter.ConsoleWriter
		w.HideCursor()
		w.CursorGoTo(2, 1)
		w.EraseDown()
		for i := 0; i < contentRows; i++ {
			idx := top + i
			w.CursorGoTo(i+2, 1)
			w.EraseLine()
			if idx >= len(summaries) {
				continue
			}
			prefix := "  "
			if idx == selected {
				prefix = "> "
			}
			w.WriteStr(truncateRunes(prefix+formatOutputSummary(summaries[idx]), r.statusWriter.cols))
		}
		r.renderManagedCommandDropdown(w, command, dropdownRow)
		r.renderManagedCommandLine(w, command, commandRow)
		r.statusWriter.renderStatusLine()
		w.ShowCursor()
		return w.Flush()
	}

	if err := render(); err != nil {
		return err
	}
	for {
		for _, event := range readManagedKeyEvents(tty, keyParser) {
			if result, handled := command.Handle(event); handled {
				switch {
				case result.cancelled:
					if err := render(); err != nil {
						return err
					}
				case result.changed:
					if result.refreshSuggestions {
						r.updateManagedCommandSuggestions(command, managedUIContextPicker)
					}
					if err := render(); err != nil {
						return err
					}
				case result.submitted != "":
					action := r.handleManagedUICommand(result.submitted, managedUIContextPicker)
					switch action.kind {
					case managedUICommandActionShowHelp:
						if err := r.showHelpView(); err != nil {
							return err
						}
						if err := render(); err != nil {
							return err
						}
					case managedUICommandActionShowOutputs:
						if err := render(); err != nil {
							return err
						}
					case managedUICommandActionShowEntry:
						if action.entry != nil {
							if err := r.showScrollView(action.entry.Spool, maxInt(0, action.entry.Spool.LineCount()-r.visibleRows())); err != nil {
								return err
							}
						}
						if err := render(); err != nil {
							return err
						}
					case managedUICommandActionNone:
						if err := render(); err != nil {
							return err
						}
						if action.message != "" {
							r.modeStatus.SetMessage(action.message)
							_ = r.statusWriter.Flush()
						}
					}
				}
				continue
			}

			switch {
			case event.Key == prompt.ControlC || event.Key == prompt.Escape || event.Text == "q":
				r.modeStatus.Clear()
				if restore := r.restoreSpool(nil); restore != nil {
					if err := r.renderLiveTail(restore); err != nil {
						return err
					}
				}
				_ = r.statusWriter.Flush()
				return nil
			case event.Key == prompt.Enter || event.Key == prompt.ControlM || event.Key == prompt.ControlJ:
				r.modeStatus.Clear()
				_ = r.statusWriter.Flush()
				entry := r.history.Get(summaries[selected].ID)
				if entry == nil {
					return nil
				}
				return r.showScrollView(entry.Spool, maxInt(0, entry.Spool.LineCount()-r.visibleRows()))
			case event.Text == "w":
				entry := r.history.Get(summaries[selected].ID)
				if entry != nil {
					path := defaultOutputSavePath()
					if err := saveSpool(entry.Spool, path); err == nil {
						r.modeStatus.SetMessage("SAVED " + path)
						_ = r.statusWriter.Flush()
					}
				}
			default:
				next := handleSelectionKey(event, selected, len(summaries), r.visibleRows())
				if next != selected {
					selected = next
					if err := render(); err != nil {
						return err
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *managedOutputRunner) restoreSpool(current *outputSpool) *outputSpool {
	if r.history == nil {
		return current
	}
	latest := r.history.Latest()
	if latest == nil || latest.Spool == nil || latest.Spool.LineCount() == 0 {
		return current
	}
	return latest.Spool
}

func (r *managedOutputRunner) showHelpView() error {
	spool, err := newOutputSpool()
	if err != nil {
		return err
	}
	defer spool.Close()

	if err := spool.Append([]byte(cheatSheetText())); err != nil {
		return err
	}
	return r.showScrollView(spool, 0)
}

func shouldOfferCompletedScroll(spool *outputSpool, visibleRows int) bool {
	return spool.LineCount() > maxInt(1, visibleRows)
}

func (r *managedOutputRunner) dropdownHeight(command *managedCommandState, availableRows int) int {
	if command == nil || !command.Active() || len(command.Suggestions()) == 0 {
		return 0
	}
	return minInt(command.DropdownHeight(), maxInt(0, availableRows))
}

func (r *managedOutputRunner) renderManagedCommandLine(w prompt.ConsoleWriter, command *managedCommandState, row int) {
	if command == nil || !command.Active() || row < 1 {
		return
	}

	w.CursorGoTo(row, 1)
	w.EraseLine()
	input := truncateRunes(command.Input(), r.statusWriter.cols)
	w.WriteStr(input)

	cursorCol := runewidth.StringWidth(input) + 1
	if cursorCol < 1 {
		cursorCol = 1
	}
	if r.statusWriter.cols > 0 {
		cursorCol = minInt(cursorCol, r.statusWriter.cols)
	}
	w.CursorGoTo(row, cursorCol)
}

func (r *managedOutputRunner) renderManagedCommandDropdown(w prompt.ConsoleWriter, command *managedCommandState, startRow int) {
	if command == nil || !command.Active() || len(command.Suggestions()) == 0 {
		return
	}

	suggestions := command.VisibleSuggestions()
	if len(suggestions) == 0 {
		return
	}

	formatted, _ := formatManagedSuggestions(suggestions, r.statusWriter.cols-1)
	selected := command.selected - command.verticalScroll
	for i := 0; i < len(formatted); i++ {
		w.CursorGoTo(startRow+i, 1)
		w.EraseLine()
		if i == selected {
			w.SetColor(prompt.Black, prompt.Cyan, true)
		} else {
			w.SetColor(prompt.White, prompt.Cyan, false)
		}
		w.WriteStr(formatted[i].Text)
		if i == selected {
			w.SetColor(prompt.Black, prompt.Cyan, false)
		} else {
			w.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
		}
		w.WriteStr(formatted[i].Description)
		w.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
	}
}

func normalizePromptCursor(spool *outputSpool) error {
	hasOutput, endedWithNewline := spool.OutputState()
	if hasOutput && !endedWithNewline {
		if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	_, err := os.Stdout.Write([]byte("\r"))
	return err
}

type terminalOutputWriter struct {
	w         io.Writer
	lastWasCR bool
}

func (w *terminalOutputWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	converted := make([]byte, 0, len(data)+bytesCount(data, '\n'))
	for _, b := range data {
		switch b {
		case '\n':
			if !w.lastWasCR {
				converted = append(converted, '\r')
			}
			converted = append(converted, '\n')
			w.lastWasCR = false
		case '\r':
			converted = append(converted, '\r')
			w.lastWasCR = true
		default:
			converted = append(converted, b)
			w.lastWasCR = false
		}
	}
	if _, err := w.w.Write(converted); err != nil {
		return 0, err
	}
	return len(data), nil
}

func bytesCount(data []byte, needle byte) int {
	var count int
	for _, b := range data {
		if b == needle {
			count++
		}
	}
	return count
}

func shouldManageCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	if usesStdinFileArg(fields) || usesTTYFlag(fields) {
		return false
	}
	switch fields[0] {
	case "exec", "attach", "edit", "port-forward", "proxy":
		return false
	default:
		return true
	}
}

func isStreamingCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		switch {
		case field == "-w" || field == "--watch" || field == "--watch=true":
			return true
		case strings.HasPrefix(field, "--watch=") && field != "--watch=false":
			return true
		case fields[0] == "logs" && (field == "-f" || field == "--follow" || field == "--follow=true"):
			return true
		case fields[0] == "logs" && strings.HasPrefix(field, "--follow=") && field != "--follow=false":
			return true
		}
	}
	return false
}

func usesStdinFileArg(fields []string) bool {
	for i, field := range fields {
		if field == "-" && i > 0 && (fields[i-1] == "-f" || fields[i-1] == "--filename") {
			return true
		}
		if field == "-f-" || field == "-f=-" || field == "--filename=-" {
			return true
		}
	}
	return false
}

func usesTTYFlag(fields []string) bool {
	for _, field := range fields {
		switch field {
		case "-i", "-t", "-it", "-ti", "--stdin", "--tty", "--stdin=true", "--tty=true":
			return true
		}
	}
	return false
}

func runDirectCommand(_ string, cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func handleScrollKey(event prompt.KeyEvent, topLine, totalLines, visibleRows int) int {
	maxTop := maxInt(0, totalLines-visibleRows)
	switch event.Key {
	case prompt.Up:
		topLine--
	case prompt.Down:
		topLine++
	case prompt.PageUp:
		topLine -= maxInt(1, visibleRows)
	case prompt.PageDown:
		topLine += maxInt(1, visibleRows)
	case prompt.Home:
		topLine = 0
	case prompt.End:
		topLine = maxTop
	}
	return clampInt(topLine, 0, maxTop)
}

func handleSelectionKey(event prompt.KeyEvent, selected, total, visibleRows int) int {
	if total <= 0 {
		return 0
	}
	switch event.Key {
	case prompt.Up:
		selected--
	case prompt.Down:
		selected++
	case prompt.PageUp:
		selected -= maxInt(1, visibleRows)
	case prompt.PageDown:
		selected += maxInt(1, visibleRows)
	case prompt.Home:
		selected = 0
	case prompt.End:
		selected = total - 1
	}
	return clampInt(selected, 0, total-1)
}

func (r *managedOutputRunner) visibleRows() int {
	if !r.statusWriter.refreshSize() {
		return 1
	}
	return maxInt(1, r.statusWriter.rows-1)
}

func (r *managedOutputRunner) liveTailRows() int {
	if !r.statusWriter.refreshSize() {
		return 1
	}
	return maxInt(1, r.statusWriter.rows-2)
}

func (r *managedOutputRunner) updateScrollStatus(topLine int, spool *outputSpool) {
	total := spool.LineCount()
	line := 0
	if total > 0 {
		line = clampInt(topLine+1, 1, total)
	}
	r.modeStatus.SetScroll(line, total)
}

func (r *managedOutputRunner) renderScrollView(spool *outputSpool, topLine int) error {
	return r.renderScrollViewState(spool, topLine, nil)
}

func (r *managedOutputRunner) renderScrollViewState(spool *outputSpool, topLine int, command *managedCommandState) error {
	if !r.statusWriter.refreshSize() {
		return nil
	}
	bodyRows := r.visibleRows()
	commandRow := 0
	if command != nil && command.Active() {
		commandRow = r.statusWriter.rows
		bodyRows = maxInt(0, bodyRows-1)
	}
	dropdownHeight := r.dropdownHeight(command, bodyRows)
	contentRows := maxInt(0, bodyRows-dropdownHeight)
	dropdownRow := 2 + contentRows
	r.updateScrollStatus(topLine, spool)

	w := r.statusWriter.ConsoleWriter
	w.HideCursor()
	w.CursorGoTo(2, 1)
	w.EraseDown()
	if contentRows > 0 {
		lines, err := spool.ReadLines(topLine, contentRows)
		if err != nil {
			return err
		}
		for i := 0; i < contentRows; i++ {
			w.CursorGoTo(i+2, 1)
			w.EraseLine()
			if i < len(lines) {
				w.WriteStr(truncateRunes(lines[i], r.statusWriter.cols))
			}
		}
	}
	r.renderManagedCommandDropdown(w, command, dropdownRow)
	r.renderManagedCommandLine(w, command, commandRow)
	r.statusWriter.renderStatusLine()
	w.ShowCursor()
	return w.Flush()
}

func (r *managedOutputRunner) renderLiveTail(spool *outputSpool) error {
	return r.renderLiveTailState(spool, nil)
}

func (r *managedOutputRunner) renderLiveTailState(spool *outputSpool, command *managedCommandState) error {
	if !r.statusWriter.refreshSize() {
		return nil
	}
	visibleRows := r.liveTailRows()
	dropdownHeight := r.dropdownHeight(command, visibleRows)
	contentRows := maxInt(0, visibleRows-dropdownHeight)
	dropdownRow := 2 + contentRows
	commandRow := r.statusWriter.rows
	total := spool.LineCount()
	start := maxInt(0, total-contentRows)
	w := r.statusWriter.ConsoleWriter
	w.HideCursor()
	w.CursorGoTo(2, 1)
	w.EraseDown()
	lines := []string(nil)
	if contentRows > 0 {
		var err error
		lines, err = spool.ReadLines(start, contentRows)
		if err != nil {
			return err
		}
	}
	for i, line := range lines {
		w.CursorGoTo(i+2, 1)
		w.EraseLine()
		w.WriteStr(truncateRunes(line, r.statusWriter.cols))
	}
	r.renderManagedCommandDropdown(w, command, dropdownRow)
	if command != nil && command.Active() {
		r.renderManagedCommandLine(w, command, commandRow)
	} else if len(lines) == 0 {
		w.CursorGoTo(2, 1)
	} else {
		w.CursorGoTo(minInt(r.statusWriter.rows, 2+len(lines)), 1)
	}
	r.statusWriter.renderStatusLine()
	w.ShowCursor()
	return w.Flush()
}

func truncateRunes(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width])
}

const (
	managedShortenSuffix = "..."
	managedLeftPrefix    = " "
	managedLeftSuffix    = " "
	managedRightPrefix   = " "
	managedRightSuffix   = " "
)

func formatManagedSuggestions(suggestions []prompt.Suggest, max int) ([]prompt.Suggest, int) {
	formatted := make([]prompt.Suggest, len(suggestions))
	left := make([]string, len(suggestions))
	right := make([]string, len(suggestions))

	for i := range suggestions {
		left[i] = suggestions[i].Text
		right[i] = suggestions[i].Description
	}

	left, leftWidth := formatManagedSuggestionTexts(left, max, managedLeftPrefix, managedLeftSuffix)
	if leftWidth == 0 {
		return nil, 0
	}
	right, rightWidth := formatManagedSuggestionTexts(right, max-leftWidth, managedRightPrefix, managedRightSuffix)
	for i := range suggestions {
		formatted[i] = prompt.Suggest{Text: left[i], Description: right[i]}
	}
	return formatted, leftWidth + rightWidth
}

func formatManagedSuggestionTexts(items []string, max int, prefix, suffix string) ([]string, int) {
	formatted := make([]string, len(items))
	width := 0
	prefixWidth := runewidth.StringWidth(prefix)
	suffixWidth := runewidth.StringWidth(suffix)
	shortenWidth := runewidth.StringWidth(managedShortenSuffix)
	minWidth := prefixWidth + suffixWidth + shortenWidth

	for i := range items {
		items[i] = strings.ReplaceAll(items[i], "\n", "")
		items[i] = strings.ReplaceAll(items[i], "\r", "")
		width = maxInt(width, runewidth.StringWidth(items[i]))
	}

	if width == 0 || minWidth >= max {
		return formatted, 0
	}
	if prefixWidth+width+suffixWidth > max {
		width = max - prefixWidth - suffixWidth
	}

	for i := range items {
		itemWidth := runewidth.StringWidth(items[i])
		if itemWidth <= width {
			formatted[i] = prefix + items[i] + strings.Repeat(" ", width-itemWidth) + suffix
			continue
		}
		formatted[i] = prefix + runewidth.FillRight(runewidth.Truncate(items[i], width, managedShortenSuffix), width) + suffix
	}
	return formatted, prefixWidth + width + suffixWidth
}

func endsWithWhitespace(s string) bool {
	if s == "" {
		return false
	}
	runes := []rune(s)
	return unicode.IsSpace(runes[len(runes)-1])
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func strconvItoa(v int) string {
	return strconv.FormatInt(int64(v), 10)
}
