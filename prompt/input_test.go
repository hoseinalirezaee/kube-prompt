package prompt

import (
	"testing"
)

func TestPosixParserGetKey(t *testing.T) {
	scenarioTable := []struct {
		name     string
		input    []byte
		expected Key
	}{
		{
			name:     "escape",
			input:    []byte{0x1b},
			expected: Escape,
		},
		{
			name:     "undefined",
			input:    []byte{'a'},
			expected: NotDefined,
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
	}

	for _, s := range scenarioTable {
		t.Run(s.name, func(t *testing.T) {
			key := GetKey(s.input)
			if key != s.expected {
				t.Errorf("Should be %s, but got %s", key, s.expected)
			}
		})
	}
}
