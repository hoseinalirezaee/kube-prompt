package kube

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
)

const bashCompletionTimeout = 500 * time.Millisecond

type shellCompleter interface {
	Complete(segment string) []prompt.Suggest
}

type bashShellCompleter struct{}

func newBashShellCompleter() shellCompleter {
	return bashShellCompleter{}
}

func (bashShellCompleter) Complete(segment string) []prompt.Suggest {
	words, cword := shellWordsForCompletion(segment)
	if cword == 0 {
		return commandSuggestions(words[cword])
	}

	suggestions := bashCompletionSuggestions(segment, words, cword)
	if len(suggestions) > 0 {
		return suggestions
	}
	return pathSuggestions(words[cword])
}

func textAfterLastShellPipe(text string) (string, bool) {
	_, after, ok := splitLastShellPipe(text)
	return after, ok
}

func splitLastShellPipe(text string) (before, after string, ok bool) {
	lastPipe := -1
	inSingle := false
	inDouble := false
	escaped := false

	for i, r := range text {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '|':
			if !inSingle && !inDouble {
				lastPipe = i
			}
		}
	}

	if lastPipe < 0 {
		return "", "", false
	}
	return text[:lastPipe], text[lastPipe+1:], true
}

func splitFirstShellPipe(text string) (before, after string, ok bool) {
	inSingle := false
	inDouble := false
	escaped := false

	for i, r := range text {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '|':
			if !inSingle && !inDouble {
				return text[:i], text[i+1:], true
			}
		}
	}

	return "", "", false
}

func shellPipeSegments(text string) []string {
	var segments []string
	start := 0
	inSingle := false
	inDouble := false
	escaped := false

	for i, r := range text {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '|':
			if !inSingle && !inDouble {
				segments = append(segments, text[start:i])
				start = i + 1
			}
		}
	}

	segments = append(segments, text[start:])
	return segments
}

func shellWordsForCompletion(segment string) ([]string, int) {
	var words []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	inWord := false
	trailingSpace := false

	for _, r := range segment {
		if escaped {
			current.WriteRune(r)
			inWord = true
			escaped = false
			trailingSpace = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			inWord = true
			trailingSpace = false
			continue
		}
		if unicode.IsSpace(r) && !inSingle && !inDouble {
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
			trailingSpace = true
			continue
		}
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				inWord = true
				trailingSpace = false
				continue
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
				inWord = true
				trailingSpace = false
				continue
			}
		}
		current.WriteRune(r)
		inWord = true
		trailingSpace = false
	}

	if inWord {
		words = append(words, current.String())
	}
	if len(words) == 0 || trailingSpace {
		words = append(words, "")
	}
	return words, len(words) - 1
}

func bashCompletionSuggestions(segment string, words []string, cword int) []prompt.Suggest {
	if _, err := exec.LookPath("bash"); err != nil {
		return nil
	}

	args := []string{
		"-lc",
		bashCompletionScript,
		"kube-prompt-completion",
		segment,
		strconv.Itoa(cword),
	}
	args = append(args, words...)

	ctx, cancel := context.WithTimeout(context.Background(), bashCompletionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", args...)
	out, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		return nil
	}
	return suggestionsFromLines(string(out))
}

func commandSuggestions(prefix string) []prompt.Suggest {
	suggestions := suggestionsFromStrings(commandNames(prefix), "command")
	if strings.HasPrefix(SecretDecodeCommand, prefix) && !containsSuggestionText(suggestions, SecretDecodeCommand) {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        SecretDecodeCommand,
			Description: "Decode Kubernetes Secret data",
		})
		sort.Slice(suggestions, func(i, j int) bool {
			return suggestions[i].Text < suggestions[j].Text
		})
	}
	return suggestions
}

func containsSuggestionText(suggestions []prompt.Suggest, text string) bool {
	for _, suggestion := range suggestions {
		if suggestion.Text == text {
			return true
		}
	}
	return false
}

func commandNames(prefix string) []string {
	names := make(map[string]struct{})
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.IsDir() || info.Mode().Perm()&0111 == 0 {
				continue
			}
			names[name] = struct{}{}
		}
	}
	return sortedKeys(names)
}

func pathSuggestions(prefix string) []prompt.Suggest {
	dir, base := filepath.Split(prefix)
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions []prompt.Suggest
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		text := filepath.Join(dir, name)
		if entry.IsDir() {
			text += string(os.PathSeparator)
		}
		suggestions = append(suggestions, prompt.Suggest{Text: text})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}

func suggestionsFromLines(output string) []prompt.Suggest {
	return suggestionsFromStrings(strings.Split(output, "\n"), "")
}

func suggestionsFromStrings(items []string, description string) []prompt.Suggest {
	seen := make(map[string]struct{}, len(items))
	suggestions := make([]prompt.Suggest, 0, len(items))
	for _, item := range items {
		item = strings.TrimRight(item, "\r")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		suggestions = append(suggestions, prompt.Suggest{Text: item, Description: description})
	}
	return suggestions
}

func sortedKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for item := range items {
		keys = append(keys, item)
	}
	sort.Strings(keys)
	return keys
}

const bashCompletionScript = `
line=$1
cword=$2
shift 2

source /usr/share/bash-completion/bash_completion >/dev/null 2>&1 || source /etc/bash_completion >/dev/null 2>&1 || true

COMP_LINE=$line
COMP_POINT=${#line}
COMP_WORDS=("$@")
COMP_CWORD=$cword
COMP_TYPE=9
COMPREPLY=()

cmd=${COMP_WORDS[0]}
cur=${COMP_WORDS[$COMP_CWORD]}
prev=
if (( COMP_CWORD > 0 )); then
	prev=${COMP_WORDS[$((COMP_CWORD - 1))]}
fi

if declare -F _completion_loader >/dev/null 2>&1; then
	_completion_loader "$cmd" >/dev/null 2>&1 || true
fi
if declare -F __load_completion >/dev/null 2>&1; then
	__load_completion "$cmd" >/dev/null 2>&1 || true
fi

spec=$(complete -p -- "$cmd" 2>/dev/null || true)
if [[ $spec =~ (^|[[:space:]])-F[[:space:]]+([^[:space:]]+) ]]; then
	fn=${BASH_REMATCH[2]}
	if declare -F "$fn" >/dev/null 2>&1; then
		"$fn" "$cmd" "$cur" "$prev" >/dev/null 2>&1 || true
		if ((${#COMPREPLY[@]} > 0)); then
			printf '%s\n' "${COMPREPLY[@]}"
			exit 0
		fi
	fi
fi

compgen -f -- "$cur"
`
