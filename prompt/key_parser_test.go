package prompt

import (
	"testing"
)

func TestKeyParser_Feed(t *testing.T) {
	parser := NewKeyParser()

	// Test ControlC
	input := []byte{0x03}
	events := parser.Feed(input)
	if len(events) != 1 || events[0].Key != ControlC {
		t.Errorf("Expected ControlC, got %+v", events)
	}

	// Test Up Arrow
	input = []byte{0x1b, 0x5b, 0x41}
	events = parser.Feed(input)
	if len(events) != 1 || events[0].Key != Up {
		t.Errorf("Expected Up, got %+v", events)
	}

	// Test unknown sequence (should fallback to text)
	input = []byte{0xff}
	events = parser.Feed(input)
	if len(events) != 1 || events[0].Key != NotDefined || events[0].Text != string([]byte{0xff}) {
		t.Errorf("Expected NotDefined with text, got %+v", events)
	}

	// Test multiple keys in one input
	input = []byte{0x03, 0x1b, 0x5b, 0x41}
	events = parser.Feed(input)
	if len(events) != 2 || events[0].Key != ControlC || events[1].Key != Up {
		t.Errorf("Expected ControlC and Up, got %+v", events)
	}

	// Test custom sequence
	parser.matcher.Insert([]byte("gg"), F24)
	input = []byte("gg")
	events = parser.Feed(input)
	if len(events) != 1 || events[0].Key != F24 {
		t.Errorf("Expected F24 for 'gg', got %+v", events)
	}
}

func TestKeyParser_HomeEndSequences(t *testing.T) {
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := NewKeyParser()
			events := parser.Feed(test.input)
			if len(events) != 1 || events[0].Key != test.expected {
				t.Fatalf("expected %s, got %+v", test.expected, events)
			}
		})
	}
}
