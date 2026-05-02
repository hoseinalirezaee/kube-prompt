package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

type outputHistory struct {
	mu         sync.Mutex
	entries    []*outputEntry
	continuous *outputSpool
	nextID     int
}

type outputEntry struct {
	ID        int
	Command   string
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
	Spool     *outputSpool
}

type outputEntrySummary struct {
	ID        int
	Index     int
	Command   string
	LineCount int
	Status    string
}

func newOutputHistory() (*outputHistory, error) {
	spool, err := newOutputSpool()
	if err != nil {
		return nil, err
	}
	return &outputHistory{
		continuous: spool,
		nextID:     1,
	}, nil
}

func (h *outputHistory) Add(command string, startedAt time.Time, err error, spool *outputSpool) (*outputEntry, error) {
	if spool == nil {
		return nil, errors.New("missing output spool")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	entry := &outputEntry{
		ID:        h.nextID,
		Command:   command,
		StartedAt: startedAt,
		EndedAt:   time.Now(),
		Err:       err,
		Spool:     spool,
	}
	h.nextID++
	h.entries = append(h.entries, entry)

	if err := h.appendContinuousLocked(entry); err != nil {
		h.entries = h.entries[:len(h.entries)-1]
		h.nextID--
		return nil, err
	}
	return entry, nil
}

func (h *outputHistory) Continuous() *outputSpool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.continuous
}

func (h *outputHistory) Latest() *outputEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 {
		return nil
	}
	return h.entries[len(h.entries)-1]
}

func (h *outputHistory) Get(id int) *outputEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, entry := range h.entries {
		if entry.ID == id {
			return entry
		}
	}
	return nil
}

func (h *outputHistory) GetNewest(index int) *outputEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	if index <= 0 || index > len(h.entries) {
		return nil
	}
	return h.entries[len(h.entries)-index]
}

func (h *outputHistory) SummariesNewestFirst() []outputEntrySummary {
	h.mu.Lock()
	defer h.mu.Unlock()

	summaries := make([]outputEntrySummary, 0, len(h.entries))
	for i, displayIndex := len(h.entries)-1, 1; i >= 0; i, displayIndex = i-1, displayIndex+1 {
		entry := h.entries[i]
		summaries = append(summaries, outputEntrySummary{
			ID:        entry.ID,
			Index:     displayIndex,
			Command:   entry.Command,
			LineCount: entry.Spool.LineCount(),
			Status:    entryStatus(entry.Err),
		})
	}
	return summaries
}

func (h *outputHistory) SaveAll(path string) error {
	spool := h.Continuous()
	if spool == nil {
		return errors.New("no output history")
	}
	return spool.SaveTo(path)
}

func (h *outputHistory) SaveID(id int, path string) error {
	entry := h.Get(id)
	if entry == nil {
		return fmt.Errorf("output %d not found", id)
	}
	return entry.Spool.SaveTo(path)
}

func (h *outputHistory) SaveLatest(path string) error {
	entry := h.Latest()
	if entry == nil {
		return errors.New("no output history")
	}
	return entry.Spool.SaveTo(path)
}

func (h *outputHistory) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var err error
	for _, entry := range h.entries {
		if closeErr := entry.Spool.Close(); err == nil {
			err = closeErr
		}
	}
	h.entries = nil
	if h.continuous != nil {
		if closeErr := h.continuous.Close(); err == nil {
			err = closeErr
		}
		h.continuous = nil
	}
	return err
}

func (h *outputHistory) appendContinuousLocked(entry *outputEntry) error {
	if h.continuous == nil {
		return errors.New("continuous output spool is closed")
	}
	header := fmt.Sprintf("\n--- command %d | %s | %s ---\n", entry.ID, entryStatus(entry.Err), entry.Command)
	if err := h.continuous.Append([]byte(header)); err != nil {
		return err
	}
	return entry.Spool.WriteTo(h.continuous)
}

func formatOutputSummaries(summaries []outputEntrySummary) string {
	if len(summaries) == 0 {
		return "No output history\n"
	}
	var b strings.Builder
	for _, summary := range summaries {
		fmt.Fprintf(&b, "%d\t%s\t%d lines\t%s\tid:%d\n", summary.Index, summary.Status, summary.LineCount, summary.Command, summary.ID)
	}
	return b.String()
}

func formatOutputSummary(summary outputEntrySummary) string {
	return fmt.Sprintf("%d  %s  %d lines  %s  id:%d", summary.Index, summary.Status, summary.LineCount, summary.Command, summary.ID)
}

func entryStatus(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}

func defaultOutputSavePath() string {
	return "kube-prompt-output-" + time.Now().Format("20060102-150405") + ".log"
}

func parseOutputID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid output id: %s", s)
	}
	return id, nil
}

func saveSpool(spool *outputSpool, path string) error {
	if spool == nil {
		return errors.New("no output selected")
	}
	if strings.TrimSpace(path) == "" {
		path = defaultOutputSavePath()
	}
	return spool.SaveTo(path)
}

func printOutputCommandUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: /output save all|last|ID PATH")
}
