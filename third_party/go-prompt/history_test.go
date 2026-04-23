package prompt

import (
	"reflect"
	"testing"
)

func TestHistoryClear(t *testing.T) {
	h := NewHistory()
	h.Add("foo")
	h.Clear()
	expected := &History{
		histories: []string{"foo"},
		tmp:       []string{"foo", ""},
		selected:  1,
	}
	if !reflect.DeepEqual(expected, h) {
		t.Errorf("Should be %#v, but got %#v", expected, h)
	}
}

func TestHistoryAdd(t *testing.T) {
	h := NewHistory()
	h.Add("echo 1")
	expected := &History{
		histories: []string{"echo 1"},
		tmp:       []string{"echo 1", ""},
		selected:  0,
	}
	if !reflect.DeepEqual(h, expected) {
		t.Errorf("Should be %v, but got %v", expected, h)
	}
}

func TestHistoryAddIgnoresConsecutiveDuplicates(t *testing.T) {
	h := NewHistory()
	for _, input := range []string{
		"get pod a",
		"get pod b",
		"get pod b",
		"get pod b",
		"get pod c",
		"get pod c",
		"get pod b",
		"get pod b",
		"get pod d",
		"get pod b",
	} {
		h.Add(input)
	}

	expected := []string{
		"get pod a",
		"get pod b",
		"get pod c",
		"get pod b",
		"get pod d",
		"get pod b",
	}
	if !reflect.DeepEqual(h.histories, expected) {
		t.Errorf("Should be %#v, but got %#v", expected, h.histories)
	}
}

func TestHistoryAddIgnoresWhitespaceOnlyInput(t *testing.T) {
	h := NewHistory()
	h.Add("get pod a")
	h.Add("   ")

	expected := []string{"get pod a"}
	if !reflect.DeepEqual(h.histories, expected) {
		t.Errorf("Should be %#v, but got %#v", expected, h.histories)
	}
}

func TestHistoryAddTrimsConsecutiveDuplicates(t *testing.T) {
	h := NewHistory()
	h.Add("get pod b")
	h.Add("  get pod b  ")

	expected := []string{"get pod b"}
	if !reflect.DeepEqual(h.histories, expected) {
		t.Errorf("Should be %#v, but got %#v", expected, h.histories)
	}
}

func TestHistoryOlder(t *testing.T) {
	h := NewHistory()
	h.Add("echo 1")

	// Prepare buffer
	buf := NewBuffer()
	buf.InsertText("echo 2", false, true)

	// [1 time] Call Older function
	buf1, changed := h.Older(buf)
	if changed {
		t.Error("Should be not changed history but changed.")
	}
	if buf1.Text() != "echo 2" {
		t.Errorf("Should be %#v, but got %#v", "echo 2", buf1.Text())
	}
}

func TestHistoryOlderSkipsJustExecutedCommand(t *testing.T) {
	h := NewHistory()
	h.Add("echo 1")
	h.Add("echo 2")

	buf := NewBuffer()
	buf1, changed := h.Older(buf)
	if !changed {
		t.Error("Should be changed history but not changed.")
	}
	if buf1.Text() != "echo 1" {
		t.Errorf("Should be %#v, but got %#v", "echo 1", buf1.Text())
	}

	// [2 times] Call Older function
	buf = NewBuffer()
	buf.InsertText("echo 1", false, true)
	buf2, changed := h.Older(buf)
	if changed {
		t.Error("Should be not changed history but changed.")
	}
	if !reflect.DeepEqual("echo 1", buf2.Text()) {
		t.Errorf("Should be %#v, but got %#v", "echo 1", buf2.Text())
	}
}

func TestHistoryOlderSkipsConsecutiveDuplicateCommands(t *testing.T) {
	h := NewHistory()
	h.Add("get pod a")
	h.Add("get pod b")
	h.Add("get pod b")
	h.Add("get pod b")

	buf := NewBuffer()
	got, changed := h.Older(buf)
	if !changed {
		t.Error("Should be changed history but not changed.")
	}
	if got.Text() != "get pod a" {
		t.Errorf("Should be %#v, but got %#v", "get pod a", got.Text())
	}
}

func TestHistoryNewerReturnsToCurrentPrompt(t *testing.T) {
	h := NewHistory()
	h.Add("echo 1")
	h.Add("echo 2")

	buf, changed := h.Older(NewBuffer())
	if !changed {
		t.Fatal("Should be changed history but not changed.")
	}

	buf, changed = h.Newer(buf)
	if !changed {
		t.Fatal("Should be changed history but not changed.")
	}
	if buf.Text() != "" {
		t.Errorf("Should be %#v, but got %#v", "", buf.Text())
	}
}

func TestHistoryNewerReturnsToCurrentPromptAfterSkippedDuplicate(t *testing.T) {
	h := NewHistory()
	h.Add("get pod a")
	h.Add("get pod b")
	h.Add("get pod b")
	h.Add("get pod b")

	buf, changed := h.Older(NewBuffer())
	if !changed {
		t.Fatal("Should be changed history but not changed.")
	}
	if buf.Text() != "get pod a" {
		t.Fatalf("Should be %#v, but got %#v", "get pod a", buf.Text())
	}

	buf, changed = h.Newer(buf)
	if !changed {
		t.Fatal("Should be changed history but not changed.")
	}
	if buf.Text() != "" {
		t.Errorf("Should be %#v, but got %#v", "", buf.Text())
	}
}
