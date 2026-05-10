package prompt

import (
	"testing"
)

func TestExactMatch(t *testing.T) {
	matcher := NewSequenceMatcher()

	// Control characters
	result, key := matcher.MatchSequence([]byte{0x03})
	if result != Exact || key == nil || *key != ControlC {
		t.Errorf("Expected Exact ControlC, got %v %v", result, key)
	}
	result, key = matcher.MatchSequence([]byte{0x1b})
	if result != Exact || key == nil || *key != Escape {
		t.Errorf("Expected Exact Escape, got %v %v", result, key)
	}

	// Arrow keys
	result, key = matcher.MatchSequence([]byte{0x1b, 0x5b, 0x41})
	if result != Exact || key == nil || *key != Up {
		t.Errorf("Expected Exact Up, got %v %v", result, key)
	}
	result, key = matcher.MatchSequence([]byte{0x1b, 0x5b, 0x42})
	if result != Exact || key == nil || *key != Down {
		t.Errorf("Expected Exact Down, got %v %v", result, key)
	}
}

func TestPrefixMatch(t *testing.T) {
	matcher := NewSequenceMatcher()
	result, _ := matcher.MatchSequence([]byte{0x1b, 0x5b})
	if result != Prefix {
		t.Errorf("Expected Prefix for ESC[ got %v", result)
	}
	result, _ = matcher.MatchSequence([]byte{0x1b, 0x4f})
	if result != Prefix {
		t.Errorf("Expected Prefix for ESC O got %v", result)
	}
}

func TestNoMatch(t *testing.T) {
	matcher := NewSequenceMatcher()
	result, _ := matcher.MatchSequence([]byte{0xff})
	if result != NoMatch {
		t.Errorf("Expected NoMatch for 0xff got %v", result)
	}
	result, _ = matcher.MatchSequence([]byte{0x1b, 0xff})
	if result != NoMatch {
		t.Errorf("Expected NoMatch for ESC 0xff got %v", result)
	}
}

func TestLongestMatch(t *testing.T) {
	matcher := NewSequenceMatcher()
	res := matcher.FindLongestMatch([]byte{0x1b, 0x5b, 0x41, 0x42})
	if res == nil || res.Key != Up || res.ConsumedBytes != 3 {
		t.Errorf("Expected Up with 3 bytes, got %v", res)
	}
	res = matcher.FindLongestMatch([]byte{0x03, 0x1b, 0x5b})
	if res == nil || res.Key != ControlC || res.ConsumedBytes != 1 {
		t.Errorf("Expected ControlC with 1 byte, got %v", res)
	}
	res = matcher.FindLongestMatch([]byte{0xff, 0xfe})
	if res != nil {
		t.Errorf("Expected nil for no match, got %v", res)
	}
}

func TestCustomSequence(t *testing.T) {
	matcher := NewSequenceMatcher()
	matcher.Insert([]byte("gg"), F24)
	result, _ := matcher.MatchSequence([]byte("g"))
	if result != Prefix {
		t.Errorf("Expected Prefix for 'g', got %v", result)
	}
	result, key := matcher.MatchSequence([]byte("gg"))
	if result != Exact || key == nil || *key != F24 {
		t.Errorf("Expected Exact F24 for 'gg', got %v %v", result, key)
	}
	result, _ = matcher.MatchSequence([]byte("ggg"))
	if result != NoMatch {
		t.Errorf("Expected NoMatch for 'ggg', got %v", result)
	}
}

func TestHomeEndSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected Key
	}{
		{
			name:     "home csi",
			input:    []byte{0x1b, 0x5b, 0x48},
			expected: Home,
		},
		{
			name:     "home application cursor",
			input:    []byte{0x1b, 0x4f, 0x48},
			expected: Home,
		},
		{
			name:     "end csi",
			input:    []byte{0x1b, 0x5b, 0x46},
			expected: End,
		},
		{
			name:     "end application cursor",
			input:    []byte{0x1b, 0x4f, 0x46},
			expected: End,
		},
		{
			name:     "end tilde 4",
			input:    []byte{0x1b, 0x5b, 0x34, 0x7e},
			expected: End,
		},
		{
			name:     "end tilde 8",
			input:    []byte{0x1b, 0x5b, 0x38, 0x7e},
			expected: End,
		},
	}

	matcher := NewSequenceMatcher()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, key := matcher.MatchSequence(test.input)
			if result != Exact || key == nil || *key != test.expected {
				t.Fatalf("expected exact %s, got result=%v key=%v", test.expected, result, key)
			}

			longest := matcher.FindLongestMatch(test.input)
			if longest == nil || longest.Key != test.expected || longest.ConsumedBytes != len(test.input) {
				t.Fatalf("expected longest match %s consuming %d bytes, got %+v", test.expected, len(test.input), longest)
			}
		})
	}
}
