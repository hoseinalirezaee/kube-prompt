package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
)

type outputSpool struct {
	mu               sync.Mutex
	file             *os.File
	path             string
	offsets          []int64
	size             int64
	endedWithNewline bool
}

func newOutputSpool() (*outputSpool, error) {
	file, err := os.CreateTemp("", "kube-prompt-output-*.log")
	if err != nil {
		return nil, err
	}
	return &outputSpool{
		file:    file,
		path:    file.Name(),
		offsets: []int64{0},
	}, nil
}

func (s *outputSpool) Append(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return errors.New("output spool is closed")
	}
	n, err := s.file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	data = data[:n]
	for i, b := range data {
		if b == '\n' {
			s.offsets = append(s.offsets, s.size+int64(i)+1)
		}
	}
	s.size += int64(n)
	s.endedWithNewline = data[len(data)-1] == '\n'
	return nil
}

func (s *outputSpool) Write(data []byte) (int, error) {
	if err := s.Append(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (s *outputSpool) WriteTo(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return errors.New("output spool is closed")
	}
	if s.size == 0 {
		return nil
	}
	_, err := io.Copy(w, io.NewSectionReader(s.file, 0, s.size))
	return err
}

func (s *outputSpool) SaveTo(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := s.WriteTo(file); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *outputSpool) LineCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lineCountLocked()
}

func (s *outputSpool) OutputState() (hasOutput bool, endedWithNewline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size > 0, s.endedWithNewline
}

func (s *outputSpool) lineCountLocked() int {
	if s.size == 0 {
		return 0
	}
	if s.endedWithNewline {
		return len(s.offsets) - 1
	}
	return len(s.offsets)
}

func (s *outputSpool) ReadLines(start, count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lineCount := s.lineCountLocked()
	if start < 0 {
		start = 0
	}
	if start >= lineCount {
		return nil, nil
	}
	if end := start + count; end < lineCount {
		count = end - start
	} else {
		count = lineCount - start
	}

	lines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		lineNo := start + i
		from := s.offsets[lineNo]
		to := s.size
		if lineNo+1 < len(s.offsets) {
			to = s.offsets[lineNo+1]
		}
		if to < from {
			to = from
		}
		buf := make([]byte, to-from)
		if len(buf) > 0 {
			if _, err := s.file.ReadAt(buf, from); err != nil {
				return nil, err
			}
		}
		buf = bytes.TrimRight(buf, "\r\n")
		lines = append(lines, string(buf))
	}
	return lines, nil
}

func (s *outputSpool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	if s.file != nil {
		err = s.file.Close()
		s.file = nil
	}
	if removeErr := os.Remove(s.path); err == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		err = removeErr
	}
	return err
}
