package prompt

import "testing"

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
