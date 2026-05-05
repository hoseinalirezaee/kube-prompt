package kube

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	"github.com/hoseinalirezaee/kube-prompt/prompt/completer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func NewCompleter(ctx context.Context, kubeconfig string, session *SessionState, defaultNamespace, proxyURL string) (*Completer, error) {
	if session == nil {
		session = NewSessionState("")
	}

	namespace, err := initialNamespaceFromKubeconfig(kubeconfig, defaultNamespace)
	if err != nil {
		return nil, err
	}
	session.SetNamespace(namespace)

	config, err := newRESTConfig(kubeconfig, proxyURL)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if statusError, ok := err.(*errors.StatusError); ok && statusError.Status().Code == 403 {
			namespaces = nil
		} else {
			return nil, err
		}
	}

	return &Completer{
		session:       session,
		namespaceList: namespaces,
		client:        client,
		dynamicClient: dynamicClient,
		kubeconfig:    kubeconfig,
		podCache:      newPodWatchCache(ctx, client),
	}, nil
}

type Completer struct {
	session        *SessionState
	namespaceList  *corev1.NamespaceList
	client         kubernetes.Interface
	dynamicClient  dynamic.Interface
	kubeconfig     string
	podCacheMu     sync.Mutex
	podCache       *podWatchCache
	shellCompleter shellCompleter
}

func (c *Completer) Close() {
	if c == nil {
		return
	}

	c.podCacheMu.Lock()
	podCache := c.podCache
	c.podCache = nil
	c.podCacheMu.Unlock()

	if podCache != nil {
		podCache.close()
	}
}

func (c *Completer) Complete(d prompt.Document) []prompt.Suggest {
	text := d.TextBeforeCursor()
	if text == "" {
		return []prompt.Suggest{}
	}
	if suggests, handled := c.completeSessionCommand(d); handled {
		return suggests
	}

	if segment, ok := textAfterLastShellPipe(text); ok {
		return c.completeShellSegment(segment)
	}

	args := strings.Split(text, " ")
	w := d.GetWordBeforeCursor()

	// If word before the cursor starts with "-", returns CLI flag options.
	if strings.HasPrefix(w, "-") {
		return optionCompleter(args, strings.HasPrefix(w, "--"))
	}

	// Return suggestions for option
	if suggests, found := c.completeOptionArguments(context.TODO(), d); found {
		return suggests
	}

	namespace := checkNamespaceArg(d)
	if namespace == "" {
		namespace = c.activeNamespace()
	}
	allNamespaces := checkAllNamespacesArg(d)
	commandArgs, skipNext := excludeOptions(args)
	if skipNext {
		// when type 'get pod -o ', we don't want to complete pods. we want to type 'json' or other.
		// So we need to skip argumentCompleter.
		return []prompt.Suggest{}
	}
	return c.argumentsCompleterWithScope(context.TODO(), namespace, allNamespaces, commandArgs)
}

func (c *Completer) completeShellSegment(segment string) []prompt.Suggest {
	shellCompleter := c.shellCompleter
	if shellCompleter == nil {
		shellCompleter = newBashShellCompleter()
	}
	return shellCompleter.Complete(segment)
}

func checkNamespaceArg(d prompt.Document) string {
	args := strings.Split(d.Text, " ")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--namespace" || arg == "-n":
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		case strings.HasPrefix(arg, "--namespace="):
			return strings.TrimPrefix(arg, "--namespace=")
		case strings.HasPrefix(arg, "-n="):
			return strings.TrimPrefix(arg, "-n=")
		case strings.HasPrefix(arg, "-n") && len(arg) > len("-n"):
			return strings.TrimPrefix(arg, "-n")
		}
	}
	return ""
}

func checkAllNamespacesArg(d prompt.Document) bool {
	args := strings.Split(d.Text, " ")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all-namespaces" || arg == "-A":
			return true
		case strings.HasPrefix(arg, "--all-namespaces="):
			value := strings.TrimPrefix(arg, "--all-namespaces=")
			return value == "" || strings.EqualFold(value, "true")
		}
	}
	return false
}

var sessionCommandSuggestions = []prompt.Suggest{
	{Text: "help", Description: "Show kube-prompt shortcuts and prompt commands"},
	{Text: "namespace", Description: "Show or change the active namespace for this session"},
	{Text: "output", Description: "Save captured command output"},
	{Text: "outputs", Description: "Browse captured command output"},
	{Text: "exit", Description: "Exit this program"},
	{Text: "quit", Description: "Exit this program"},
}

func (c *Completer) completeSessionCommand(d prompt.Document) ([]prompt.Suggest, bool) {
	text := d.TextBeforeCursor()
	if !strings.HasPrefix(text, "/") {
		return nil, false
	}

	args := strings.Split(text, " ")
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(sessionCommandSuggestions, strings.TrimPrefix(args[0], "/"), true), true
	}
	if args[0] == "/namespace" && len(args) == 2 {
		return prompt.FilterHasPrefix(getNameSpaceSuggestions(c.namespaceList), args[1], true), true
	}
	return []prompt.Suggest{}, true
}

func (c *Completer) activeNamespace() string {
	if c == nil || c.session == nil {
		return ""
	}
	return c.session.Namespace()
}

/* Option arguments */

var yamlFileCompleter = completer.FilePathCompleter{
	IgnoreCase: true,
	Filter: func(fi os.FileInfo) bool {
		if fi.IsDir() {
			return true
		}
		if strings.HasSuffix(fi.Name(), ".yaml") || strings.HasSuffix(fi.Name(), ".yml") {
			return true
		}
		return false
	},
}

func getPreviousOption(d prompt.Document) (cmd, option string, found bool) {
	args := strings.Split(d.TextBeforeCursor(), " ")
	l := len(args)
	if l >= 2 {
		option = args[l-2]
	}
	if strings.HasPrefix(option, "-") {
		return args[0], option, true
	}
	return "", "", false
}

func (c *Completer) completeOptionArguments(ctx context.Context, d prompt.Document) ([]prompt.Suggest, bool) {
	cmd, option, found := getPreviousOption(d)
	if !found {
		return []prompt.Suggest{}, false
	}

	// namespace
	if option == "-n" || option == "--namespace" {
		return prompt.FilterHasPrefix(
			getNameSpaceSuggestions(c.namespaceList),
			d.GetWordBeforeCursor(),
			true,
		), true
	}

	// filename
	switch cmd {
	case "get", "describe", "create", "delete", "replace", "patch",
		"edit", "apply", "expose", "rolling-update", "rollout",
		"label", "annotate", "scale", "convert", "autoscale", "top",
		"auth", "debug", "diff", "wait":
		if option == "-f" || option == "--filename" || option == "-k" || option == "--kustomize" {
			return yamlFileCompleter.Complete(d), true
		}
	}

	// container
	switch cmd {
	case "exec", "logs", "run", "attach", "port-forward", "cp", "debug":
		if option == "-c" || option == "--container" {
			cmdArgs := getCommandArgs(d)
			namespace := checkNamespaceArg(d)
			if namespace == "" {
				namespace = c.activeNamespace()
			}
			var suggestions []prompt.Suggest
			if cmdArgs == nil || len(cmdArgs) < 2 {
				suggestions = c.getContainerNamesFromCachedPods(ctx, namespace)
			} else {
				suggestions = c.getContainerName(ctx, namespace, cmdArgs[1])
			}
			return prompt.FilterHasPrefix(
				suggestions,
				d.GetWordBeforeCursor(),
				true,
			), true
		}
	}
	return []prompt.Suggest{}, false
}

func getCommandArgs(d prompt.Document) []string {
	args := strings.Split(d.TextBeforeCursor(), " ")

	// If PIPE is in text before the cursor, returns empty.
	for i := range args {
		if args[i] == "|" {
			return nil
		}
	}

	commandArgs, _ := excludeOptions(args)
	return commandArgs
}

func excludeOptions(args []string) ([]string, bool) {
	l := len(args)
	if l == 0 {
		return nil, false
	}
	cmd := args[0]
	filtered := make([]string, 0, l)

	var skipNextArg bool
	for i := 0; i < len(args); i++ {
		if skipNextArg {
			skipNextArg = false
			continue
		}

		if cmd == "logs" && args[i] == "-f" {
			continue
		}

		for _, s := range []string{
			"-f", "--filename",
			"-k", "--kustomize",
			"-n", "--namespace",
			"-s", "--server",
			"--kubeconfig",
			"--cluster",
			"--user",
			"-o", "--output",
			"-c",
			"--container",
		} {
			if strings.HasPrefix(args[i], s) {
				if strings.Contains(args[i], "=") {
					// we can specify option value like '-o=json'
					skipNextArg = false
				} else {
					skipNextArg = true
				}
				continue
			}
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}

		filtered = append(filtered, args[i])
	}
	return filtered, skipNextArg
}
