package kube

import (
	"reflect"
	"testing"
)

func TestTextAfterLastShellPipe(t *testing.T) {
	tests := []struct {
		input       string
		wantSegment string
		wantFound   bool
	}{
		{input: "get pods", wantFound: false},
		{input: "get pods | grep", wantSegment: " grep", wantFound: true},
		{input: "get pods | grep web | aw", wantSegment: " aw", wantFound: true},
		{input: `get pods | grep "a|b" | aw`, wantSegment: " aw", wantFound: true},
		{input: `get pods | grep 'a|b'`, wantSegment: " grep 'a|b'", wantFound: true},
		{input: `get pods | grep a\|b`, wantSegment: ` grep a\|b`, wantFound: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotSegment, gotFound := textAfterLastShellPipe(tt.input)
			if gotFound != tt.wantFound || gotSegment != tt.wantSegment {
				t.Fatalf("expected (%q, %v), got (%q, %v)", tt.wantSegment, tt.wantFound, gotSegment, gotFound)
			}
		})
	}
}

func TestShellWordsForCompletion(t *testing.T) {
	tests := []struct {
		input     string
		wantWords []string
		wantCWord int
	}{
		{input: "", wantWords: []string{""}, wantCWord: 0},
		{input: " ", wantWords: []string{""}, wantCWord: 0},
		{input: " gr", wantWords: []string{"gr"}, wantCWord: 0},
		{input: " grep --", wantWords: []string{"grep", "--"}, wantCWord: 1},
		{input: " grep ", wantWords: []string{"grep", ""}, wantCWord: 1},
		{input: ` grep "web pod" --`, wantWords: []string{"grep", "web pod", "--"}, wantCWord: 2},
		{input: ` grep a\ b`, wantWords: []string{"grep", "a b"}, wantCWord: 1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotWords, gotCWord := shellWordsForCompletion(tt.input)
			if !reflect.DeepEqual(gotWords, tt.wantWords) || gotCWord != tt.wantCWord {
				t.Fatalf("expected (%v, %d), got (%v, %d)", tt.wantWords, tt.wantCWord, gotWords, gotCWord)
			}
		})
	}
}
