package kube

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
)

func TestDecodeSecretData(t *testing.T) {
	input := `{
		"apiVersion": "v1",
		"kind": "Secret",
		"data": {
			"username": "ZGVtby11c2Vy",
			"password": "czNjcjN0LXBhc3M=",
			"token": "ZGVtby10b2tlbi0xMjM="
		}
	}`
	var out bytes.Buffer

	if err := DecodeSecretData(strings.NewReader(input), &out); err != nil {
		t.Fatalf("expected decode to succeed, got %v", err)
	}
	if got, want := out.String(), "password: s3cr3t-pass\ntoken: demo-token-123\nusername: demo-user\n"; got != want {
		t.Fatalf("expected decoded output %q, got %q", want, got)
	}
}

func TestDecodeSecretDataEscapesBinaryValues(t *testing.T) {
	value := []byte{0xff, 0x00, 'A'}
	input := `{"data":{"bin":"` + base64.StdEncoding.EncodeToString(value) + `"}}`
	var out bytes.Buffer

	if err := DecodeSecretData(strings.NewReader(input), &out); err != nil {
		t.Fatalf("expected decode to succeed, got %v", err)
	}
	if got, want := out.String(), "bin: "+strconv.Quote(string(value))+"\n"; got != want {
		t.Fatalf("expected decoded output %q, got %q", want, got)
	}
}

func TestDecodeSecretDataErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty input", input: "", want: "empty input"},
		{name: "invalid json", input: "{", want: "invalid Secret JSON"},
		{name: "missing data", input: `{"kind":"Secret"}`, want: "no data field"},
		{name: "bad base64", input: `{"data":{"password":"not-base64!"}}`, want: `data["password"] is not valid base64`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := DecodeSecretData(strings.NewReader(tt.input), &out)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
			if out.Len() != 0 {
				t.Fatalf("expected no output on error, got %q", out.String())
			}
		})
	}
}
