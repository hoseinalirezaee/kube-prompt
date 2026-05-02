package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestOutputHistoryKeepsContinuousAndPerCommandOutput(t *testing.T) {
	history, err := newOutputHistory()
	if err != nil {
		t.Fatalf("expected history, got error %v", err)
	}
	defer history.Close()

	first := mustSpool(t, "pod-a\n")
	second := mustSpool(t, "node-a\nnode-b\n")
	if _, err := history.Add("get pods", time.Now(), nil, first); err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	if _, err := history.Add("get nodes", time.Now(), nil, second); err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	summaries := history.SummariesNewestFirst()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %#v", summaries)
	}
	if summaries[0].Command != "get nodes" || summaries[1].Command != "get pods" {
		t.Fatalf("expected newest-first summaries, got %#v", summaries)
	}
	if summaries[0].Index != 1 || summaries[1].Index != 2 {
		t.Fatalf("expected newest-first display indexes, got %#v", summaries)
	}
	if got := history.GetNewest(1); got == nil || got.Command != "get nodes" {
		t.Fatalf("expected newest output to be get nodes, got %#v", got)
	}
	if got := history.GetNewest(2); got == nil || got.Command != "get pods" {
		t.Fatalf("expected second newest output to be get pods, got %#v", got)
	}

	lines, err := history.Continuous().ReadLines(0, 20)
	if err != nil {
		t.Fatalf("read continuous failed: %v", err)
	}
	continuous := strings.Join(lines, "\n")
	for _, want := range []string{"command 1", "get pods", "pod-a", "command 2", "get nodes", "node-b"} {
		if !strings.Contains(continuous, want) {
			t.Fatalf("expected continuous output to contain %q, got %q", want, continuous)
		}
	}
}

func TestOutputHistorySaveTargets(t *testing.T) {
	history, err := newOutputHistory()
	if err != nil {
		t.Fatalf("expected history, got error %v", err)
	}
	defer history.Close()

	first := mustSpool(t, "first\n")
	second := mustSpool(t, "second\n")
	firstEntry, err := history.Add("first command", time.Now(), nil, first)
	if err != nil {
		t.Fatalf("add first failed: %v", err)
	}
	if _, err := history.Add("second command", time.Now(), nil, second); err != nil {
		t.Fatalf("add second failed: %v", err)
	}

	dir := t.TempDir()
	allPath := dir + "/all.log"
	idPath := dir + "/id.log"
	lastPath := dir + "/last.log"
	if err := history.SaveAll(allPath); err != nil {
		t.Fatalf("save all failed: %v", err)
	}
	if err := history.SaveID(firstEntry.ID, idPath); err != nil {
		t.Fatalf("save id failed: %v", err)
	}
	if err := history.SaveLatest(lastPath); err != nil {
		t.Fatalf("save latest failed: %v", err)
	}

	assertFileContains(t, allPath, "first\n")
	assertFileContains(t, allPath, "second\n")
	assertFileContains(t, idPath, "first\n")
	assertFileContains(t, lastPath, "second\n")
}

func mustSpool(t *testing.T, data string) *outputSpool {
	t.Helper()
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("new spool failed: %v", err)
	}
	if err := spool.Append([]byte(data)); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	return spool
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(data))
	}
}
