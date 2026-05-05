package kube

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"
)

const (
	SecretDecodeCommand      = "kpb64decode"
	SecretDecodeInternalFlag = "--kube-prompt-internal-secret-decode"
)

type secretDecodeInput struct {
	Data map[string]string `json:"data"`
}

func DecodeSecretData(r io.Reader, w io.Writer) error {
	var input secretDecodeInput
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s: empty input", SecretDecodeCommand)
		}
		return fmt.Errorf("%s: invalid Secret JSON: %w", SecretDecodeCommand, err)
	}
	if input.Data == nil {
		return fmt.Errorf("%s: Secret has no data field", SecretDecodeCommand)
	}

	keys := make([]string, 0, len(input.Data))
	for key := range input.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		decoded, err := base64.StdEncoding.DecodeString(input.Data[key])
		if err != nil {
			return fmt.Errorf("%s: data[%q] is not valid base64: %w", SecretDecodeCommand, key, err)
		}
		if _, err := fmt.Fprintf(w, "%s: %s\n", key, formatDecodedSecretValue(decoded)); err != nil {
			return err
		}
	}
	return nil
}

func formatDecodedSecretValue(value []byte) string {
	if utf8.Valid(value) {
		return string(value)
	}
	return strconv.Quote(string(value))
}
