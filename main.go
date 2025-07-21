package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"text/tabwriter"
)

type podList struct {
	Items []struct {
		Metadata struct {
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			NodeName string `json:"nodeName"`
		} `json:"spec"`
	} `json:"items"`
}

type nodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	} `json:"items"`
}

func main() {
	// Parse flags to check for --all-namespaces
	var allNamespaces bool
	var kubectlArgs []string
	
	// Process args to separate our flags from kubectl flags
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--all-namespaces" || arg == "-A" {
			allNamespaces = true
			kubectlArgs = append(kubectlArgs, arg)
		} else {
			kubectlArgs = append(kubectlArgs, arg)
		}
	}

	// Forward all CLI args to the inner `kubectl get pods`
	inner := append([]string{"get", "pods"}, kubectlArgs...)
	inner = append(inner, "-o", "json")

	out, err := exec.Command("kubectl", inner...).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var pl podList
	if err := json.Unmarshal(out, &pl); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Fetch all nodes to get zone information
	nodeOut, err := exec.Command("kubectl", "get", "nodes", "-o", "json").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var nl nodeList
	if err := json.Unmarshal(nodeOut, &nl); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Create a map of node names to zones
	nodeZones := make(map[string]string)
	for _, node := range nl.Items {
		zone := node.Metadata.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = node.Metadata.Labels["failure-domain.beta.kubernetes.io/zone"]
		}
		nodeZones[node.Metadata.Name] = zone
	}

	// Prepare tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	
	// Set header based on whether we're showing all namespaces
	if allNamespaces {
		fmt.Fprintln(w, "NAMESPACE\tNAME\tZONE")
	} else {
		fmt.Fprintln(w, "NAME\tZONE")
	}

	// Collect pods with their zones
	type podEntry struct {
		namespace string
		name      string
		zone      string
	}
	
	var entries []podEntry
	for _, p := range pl.Items {
		zone := nodeZones[p.Spec.NodeName]
		entries = append(entries, podEntry{
			namespace: p.Metadata.Namespace,
			name:      p.Metadata.Name,
			zone:      zone,
		})
	}

	// Sort entries by namespace first, then by pod name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].namespace != entries[j].namespace {
			return entries[i].namespace < entries[j].namespace
		}
		return entries[i].name < entries[j].name
	})

	// Output the entries
	for _, e := range entries {
		if allNamespaces {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.namespace, e.name, e.zone)
		} else {
			fmt.Fprintf(w, "%s\t%s\n", e.name, e.zone)
		}
	}

	w.Flush()
}
