package kube

import (
	"context"

	"github.com/c-bata/go-prompt"
)

var commands = []prompt.Suggest{
	{Text: "annotate", Description: "Update the annotations on a resource"},
	{Text: "apply", Description: "Apply a configuration to a resource by file name or stdin"},
	{Text: "attach", Description: "Attach to a running container"},
	{Text: "auth", Description: "Inspect authorization"},
	{Text: "autoscale", Description: "Auto-scale a deployment, replica set, stateful set, or replication controller"},
	{Text: "certificate", Description: "Modify certificate resources"},
	{Text: "cluster-info", Description: "Display cluster information"},
	{Text: "completion", Description: "Output shell completion code for the specified shell"},
	{Text: "config", Description: "Modify kubeconfig files"},
	{Text: "cordon", Description: "Mark node as unschedulable"},
	{Text: "cp", Description: "Copy files and directories to and from containers"},
	{Text: "create", Description: "Create a resource from a file or from stdin"},
	{Text: "debug", Description: "Create debugging sessions for troubleshooting workloads and nodes"},
	{Text: "delete", Description: "Delete resources by file names, stdin, resources and names, or by resources and label selector"},
	{Text: "describe", Description: "Show details of a specific resource or group of resources"},
	{Text: "diff", Description: "Diff the live version against a would-be applied version"},
	{Text: "drain", Description: "Drain node in preparation for maintenance"},
	{Text: "edit", Description: "Edit a resource on the server"},
	{Text: "events", Description: "List events"},
	{Text: "exec", Description: "Execute a command in a container"},
	{Text: "explain", Description: "Get documentation for a resource"},
	{Text: "expose", Description: "Take a replication controller, service, deployment or pod and expose it as a new Kubernetes service"},
	{Text: "get", Description: "Display one or many resources"},
	{Text: "kustomize", Description: "Build a kustomization target from a directory or URL"},
	{Text: "label", Description: "Update the labels on a resource"},
	{Text: "logs", Description: "Print the logs for a container in a pod"},
	{Text: "patch", Description: "Update fields of a resource"},
	{Text: "plugin", Description: "Provides utilities for interacting with plugins"},
	{Text: "port-forward", Description: "Forward one or more local ports to a pod"},
	{Text: "proxy", Description: "Run a proxy to the Kubernetes API server"},
	{Text: "replace", Description: "Replace a resource by file name or stdin"},
	{Text: "rollout", Description: "Manage the rollout of a resource"},
	{Text: "run", Description: "Run a particular image on the cluster"},
	{Text: "scale", Description: "Set a new size for a deployment, replica set, or replication controller"},
	{Text: "set", Description: "Set specific features on objects"},
	{Text: "taint", Description: "Update the taints on one or more nodes"},
	{Text: "top", Description: "Display resource (CPU/memory) usage"},
	{Text: "uncordon", Description: "Mark node as schedulable"},
	{Text: "wait", Description: "Wait for a specific condition on one or many resources"},
	{Text: "api-resources", Description: "Print the supported API resources on the server"},
	{Text: "api-versions", Description: "Print the supported API versions on the server"},
	{Text: "version", Description: "Print the client and server version information"},

	// Deprecated commands still recognized by older kubectl releases.
	{Text: "convert", Description: "Convert config files between different API versions"},
	{Text: "namespace", Description: "SUPERSEDED: Set and view the current Kubernetes namespace"},
	{Text: "rolling-update", Description: "Perform a rolling update of the given ReplicationController"},

	// Custom command.
	{Text: "exit", Description: "Exit this program"},
}

var (
	authSubcommands = []prompt.Suggest{
		{Text: "can-i", Description: "Check whether an action is allowed"},
		{Text: "reconcile", Description: "Reconciles RBAC roles and bindings"},
		{Text: "whoami", Description: "Check self subject attributes"},
	}
	certificateSubcommands = []prompt.Suggest{
		{Text: "approve", Description: "Approve a certificate signing request"},
		{Text: "deny", Description: "Deny a certificate signing request"},
	}
	completionShells = []prompt.Suggest{
		{Text: "bash"},
		{Text: "fish"},
		{Text: "powershell"},
		{Text: "zsh"},
	}
	createSubcommands = []prompt.Suggest{
		{Text: "clusterrole", Description: "Create a cluster role"},
		{Text: "clusterrolebinding", Description: "Create a cluster role binding for a particular cluster role"},
		{Text: "configmap", Description: "Create a config map from a local file, directory or literal value"},
		{Text: "cronjob", Description: "Create a cron job with the specified name"},
		{Text: "deployment", Description: "Create a deployment with the specified name"},
		{Text: "ingress", Description: "Create an ingress with the specified name"},
		{Text: "job", Description: "Create a job with the specified name"},
		{Text: "namespace", Description: "Create a namespace with the specified name"},
		{Text: "poddisruptionbudget", Description: "Create a pod disruption budget with the specified name"},
		{Text: "priorityclass", Description: "Create a priority class with the specified name"},
		{Text: "quota", Description: "Create a quota with the specified name"},
		{Text: "role", Description: "Create a role with single rule"},
		{Text: "rolebinding", Description: "Create a role binding for a particular role or cluster role"},
		{Text: "secret", Description: "Create a secret using a specified subcommand"},
		{Text: "service", Description: "Create a service using a specified subcommand"},
		{Text: "serviceaccount", Description: "Create a service account with the specified name"},
		{Text: "token", Description: "Request a service account token"},
	}
	createSecretSubcommands = []prompt.Suggest{
		{Text: "docker-registry", Description: "Create a secret for use with a Docker registry"},
		{Text: "generic", Description: "Create a secret from a local file, directory, or literal value"},
		{Text: "tls", Description: "Create a TLS secret"},
	}
	createServiceSubcommands = []prompt.Suggest{
		{Text: "clusterip", Description: "Create a ClusterIP service"},
		{Text: "externalname", Description: "Create an ExternalName service"},
		{Text: "loadbalancer", Description: "Create a LoadBalancer service"},
		{Text: "nodeport", Description: "Create a NodePort service"},
	}
	pluginSubcommands = []prompt.Suggest{
		{Text: "list", Description: "List all visible plugin executables on a user's PATH"},
	}
	rolloutSubcommands = []prompt.Suggest{
		{Text: "history", Description: "View rollout history"},
		{Text: "pause", Description: "Mark the provided resource as paused"},
		{Text: "restart", Description: "Restart a resource"},
		{Text: "resume", Description: "Resume a paused resource"},
		{Text: "status", Description: "Show the status of the rollout"},
		{Text: "undo", Description: "Undo a previous rollout"},
	}
	setSubcommands = []prompt.Suggest{
		{Text: "env", Description: "Update environment variables on a pod template"},
		{Text: "image", Description: "Update the image of a pod template"},
		{Text: "resources", Description: "Update resource requests/limits on objects with pod templates"},
		{Text: "selector", Description: "Set the selector on a resource"},
		{Text: "serviceaccount", Description: "Update the service account of a resource"},
		{Text: "subject", Description: "Update the user, group, or service account in a role binding or cluster role binding"},
	}
)

func (c *Completer) argumentsCompleter(ctx context.Context, namespace string, args []string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(commands, args[0], true)
	}

	first := args[0]
	switch first {
	case "get":
		second := args[1]
		if len(args) == 2 {
			return c.completeResourceType(ctx, "get", second)
		}

		third := args[2]
		if len(args) == 3 {
			return c.completeResourceName(ctx, namespace, "get", second, third)
		}
	case "describe":
		second := args[1]
		if len(args) == 2 {
			return c.completeResourceType(ctx, "describe", second)
		}

		third := args[2]
		if len(args) == 3 {
			return c.completeResourceName(ctx, namespace, "describe", second, third)
		}
	case "create":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(createSubcommands, args[1], true)
		}
		if len(args) == 3 {
			switch args[1] {
			case "secret":
				return prompt.FilterHasPrefix(createSecretSubcommands, args[2], true)
			case "service", "svc":
				return prompt.FilterHasPrefix(createServiceSubcommands, args[2], true)
			}
		}
	case "delete":
		second := args[1]
		if len(args) == 2 {
			return c.completeResourceType(ctx, "delete", second)
		}

		third := args[2]
		if len(args) == 3 {
			return c.completeResourceName(ctx, namespace, "delete", second, third)
		}
	case "edit":
		if len(args) == 2 {
			return c.completeResourceType(ctx, "edit", args[1])
		}

		if len(args) == 3 {
			return c.completeResourceName(ctx, namespace, "edit", args[1], args[2])
		}

	case "patch", "label", "annotate":
		return c.completeTypeThenName(ctx, namespace, "patch", args)
	case "autoscale", "expose", "wait":
		return c.completeTypeThenName(ctx, namespace, "get", args)
	case "set":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(setSubcommands, args[1], true)
		}
		if len(args) >= 3 {
			return c.completeTypeThenNameWithOffset(ctx, namespace, "patch", args, 2)
		}
	case "auth":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(authSubcommands, args[1], true)
		}
		if len(args) == 4 && args[1] == "can-i" {
			return c.completeResourceType(ctx, "get", args[3])
		}
	case "certificate":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(certificateSubcommands, args[1], true)
		}
	case "completion":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(completionShells, args[1], true)
		}
	case "plugin":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(pluginSubcommands, args[1], true)
		}
	case "namespace":
		if len(args) == 2 {
			return prompt.FilterContains(getNameSpaceSuggestions(c.namespaceList), args[1], true)
		}
	case "logs":
		if len(args) == 2 {
			return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), args[1], true)
		}
	case "rolling-update", "rollingupdate":
		if len(args) == 2 {
			return prompt.FilterContains(getReplicationControllerSuggestions(ctx, c.client, namespace), args[1], true)
		} else if len(args) == 3 {
			return prompt.FilterContains(getReplicationControllerSuggestions(ctx, c.client, namespace), args[2], true)
		}
	case "scale", "resize":
		if len(args) == 2 {
			r := c.completeResourceType(ctx, "get", args[1])
			r = append(r, getDeploymentSuggestions(ctx, c.client, namespace)...)
			r = append(r, getReplicaSetSuggestions(ctx, c.client, namespace)...)
			r = append(r, getReplicationControllerSuggestions(ctx, c.client, namespace)...)
			return prompt.FilterContains(r, args[1], true)
		}
		if len(args) == 3 {
			return c.completeResourceName(ctx, namespace, "get", args[1], args[2])
		}
	case "cordon":
		fallthrough
	case "drain":
		fallthrough
	case "uncordon":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(getNodeSuggestions(ctx, c.client), args[1], true)
		}
	case "taint":
		if len(args) == 2 {
			return prompt.FilterHasPrefix([]prompt.Suggest{{Text: "node"}, {Text: "nodes"}, {Text: "no"}}, args[1], true)
		}
		if len(args) == 3 {
			return prompt.FilterContains(getNodeSuggestions(ctx, c.client), args[2], true)
		}
	case "attach":
		if len(args) == 2 {
			return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), args[1], true)
		}
	case "exec":
		if len(args) == 2 {
			return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), args[1], true)
		}
	case "debug":
		if len(args) == 2 {
			pods := getPodSuggestions(ctx, c.client, namespace)
			nodes := addPrefixToSuggestions(getNodeSuggestions(ctx, c.client), "node/")
			return prompt.FilterContains(append(pods, nodes...), args[1], true)
		}
	case "cp":
		if len(args) == 2 || len(args) == 3 {
			return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), args[len(args)-1], true)
		}
	case "port-forward":
		if len(args) == 2 {
			return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), args[1], true)
		}
		if len(args) == 3 {
			return prompt.FilterHasPrefix(getPortsFromPodName(namespace, args[1]), args[2], true)
		}
	case "rollout":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(rolloutSubcommands, args[1], true)
		}
		if len(args) >= 3 {
			return c.completeTypeThenNameWithOffset(ctx, namespace, "rollout", args, 2)
		}
	case "config":
		subCommands := []prompt.Suggest{
			{Text: "current-context", Description: "Displays the current-context"},
			{Text: "delete-cluster", Description: "Delete the specified cluster from the kubeconfig"},
			{Text: "delete-context", Description: "Delete the specified context from the kubeconfig"},
			{Text: "delete-user", Description: "Delete the specified user from the kubeconfig"},
			{Text: "get-clusters", Description: "Display clusters defined in the kubeconfig"},
			{Text: "get-contexts", Description: "Describe one or many contexts"},
			{Text: "get-users", Description: "Display users defined in the kubeconfig"},
			{Text: "rename-context", Description: "Rename a context from the kubeconfig file"},
			{Text: "set", Description: "Sets an individual value in a kubeconfig file"},
			{Text: "set-cluster", Description: "Sets a cluster entry in kubeconfig"},
			{Text: "set-context", Description: "Sets a context entry in kubeconfig"},
			{Text: "set-credentials", Description: "Sets a user entry in kubeconfig"},
			{Text: "unset", Description: "Unsets an individual value in a kubeconfig file"},
			{Text: "use-context", Description: "Sets the current-context in a kubeconfig file"},
			{Text: "view", Description: "Display merged kubeconfig settings or a specified kubeconfig file"},
		}
		if len(args) == 2 {
			return prompt.FilterHasPrefix(subCommands, args[1], true)
		}
		if len(args) == 3 {
			third := args[2]
			switch args[1] {
			case "use-context":
				return prompt.FilterContains(getContextSuggestions(c.kubeconfig), third, true)
			}
		}
	case "cluster-info":
		subCommands := []prompt.Suggest{
			{Text: "dump", Description: "Dump lots of relevant info for debugging and diagnosis"},
		}
		if len(args) == 2 {
			return prompt.FilterHasPrefix(subCommands, args[1], true)
		}
	case "explain":
		return c.completeResourceType(ctx, "explain", args[1])
	case "api-resources", "api-versions", "apply", "diff", "events", "kustomize", "proxy", "replace", "run", "version":
		return []prompt.Suggest{}
	case "top":
		second := args[1]
		if len(args) == 2 {
			subcommands := []prompt.Suggest{
				{Text: "nodes"},
				{Text: "pod"},
				// aliases
				{Text: "no"},
				{Text: "po"},
			}
			return prompt.FilterHasPrefix(subcommands, second, true)
		}

		third := args[2]
		if len(args) == 3 {
			switch second {
			case "no", "node", "nodes":
				return prompt.FilterContains(getNodeSuggestions(ctx, c.client), third, true)
			case "po", "pod", "pods":
				return prompt.FilterContains(getPodSuggestions(ctx, c.client, namespace), third, true)
			}
		}
	default:
		return []prompt.Suggest{}
	}
	return []prompt.Suggest{}
}

func (c *Completer) completeResourceType(ctx context.Context, command, word string) []prompt.Suggest {
	return prompt.FilterHasPrefix(
		getDiscoveredResourceTypeSuggestions(ctx, c.client, command),
		word,
		true,
	)
}

func (c *Completer) completeResourceName(ctx context.Context, namespace, command, resourceType, word string) []prompt.Suggest {
	return prompt.FilterContains(
		c.getResourceNameSuggestions(ctx, namespace, command, resourceType),
		word,
		true,
	)
}

func (c *Completer) completeTypeThenName(ctx context.Context, namespace, command string, args []string) []prompt.Suggest {
	return c.completeTypeThenNameWithOffset(ctx, namespace, command, args, 1)
}

func (c *Completer) completeTypeThenNameWithOffset(ctx context.Context, namespace, command string, args []string, offset int) []prompt.Suggest {
	if len(args) == offset+1 {
		return c.completeResourceType(ctx, command, args[offset])
	}
	if len(args) == offset+2 {
		return c.completeResourceName(ctx, namespace, command, args[offset], args[offset+1])
	}
	return []prompt.Suggest{}
}

func addPrefixToSuggestions(suggestions []prompt.Suggest, prefix string) []prompt.Suggest {
	prefixed := make([]prompt.Suggest, len(suggestions))
	for i := range suggestions {
		prefixed[i] = suggestions[i]
		prefixed[i].Text = prefix + suggestions[i].Text
	}
	return prefixed
}

func (c *Completer) getResourceNameSuggestions(ctx context.Context, namespace, command, resourceType string) []prompt.Suggest {
	resource, ok := resolveDiscoveredResource(ctx, c.client, command, resourceType)
	if !ok {
		return []prompt.Suggest{}
	}

	switch resource.Name {
	case "componentstatuses":
		return getComponentStatusCompletions(ctx, c.client)
	case "configmaps":
		return getConfigMapSuggestions(ctx, c.client, namespace)
	case "daemonsets":
		return getDaemonSetSuggestions(ctx, c.client, namespace)
	case "deployments":
		return getDeploymentSuggestions(ctx, c.client, namespace)
	case "endpoints":
		return getEndpointsSuggestions(ctx, c.client, namespace)
	case "events":
		return getEventsSuggestions(ctx, c.client, namespace)
	case "ingresses":
		return getIngressSuggestions(ctx, c.client, namespace)
	case "limitranges":
		return getLimitRangeSuggestions(ctx, c.client, namespace)
	case "namespaces":
		return getNameSpaceSuggestions(c.namespaceList)
	case "nodes":
		return getNodeSuggestions(ctx, c.client)
	case "pods":
		return getPodSuggestions(ctx, c.client, namespace)
	case "persistentvolumeclaims":
		return getPersistentVolumeClaimSuggestions(ctx, c.client, namespace)
	case "persistentvolumes":
		return getPersistentVolumeSuggestions(ctx, c.client)
	case "podtemplates":
		return getPodTemplateSuggestions(ctx, c.client, namespace)
	case "replicasets":
		return getReplicaSetSuggestions(ctx, c.client, namespace)
	case "replicationcontrollers":
		return getReplicationControllerSuggestions(ctx, c.client, namespace)
	case "resourcequotas":
		return getResourceQuotasSuggestions(ctx, c.client, namespace)
	case "secrets":
		return getSecretSuggestions(ctx, c.client, namespace)
	case "serviceaccounts":
		return getServiceAccountSuggestions(ctx, c.client, namespace)
	case "services":
		return getServiceSuggestions(ctx, c.client, namespace)
	case "jobs":
		return getJobSuggestions(ctx, c.client, namespace)
	default:
		return []prompt.Suggest{}
	}
}
