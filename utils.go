// Copyright © 2023 The Gomon Project.

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type (
	// tree organizes inforamtion types parsed from packages.
	tree map[string]tree
)

// add inserts a node into a tree.
func add(tr tree, nodes ...string) {
	if len(nodes) > 0 {
		if _, ok := tr[nodes[0]]; !ok {
			tr[nodes[0]] = tree{}
		}
		add(tr[nodes[0]], nodes[1:]...)
	}
}

// sortkeys sorts the keys for the top nodes of a tree and returns a slice of the keys.
func sortkeys(tr tree) []string {
	keys := make([]string, len(tr))
	i := 0
	for key := range tr {
		if key != "" {
			keys[i] = key
			i++
		}
	}
	keys = keys[0:i]

	sort.Slice(keys, func(i, j int) bool {
		keyi, keyj := keys[i], keys[j]
		keyi = strings.Trim(keyi, "*()")
		keyj = strings.Trim(keyj, "*()")
		return keyi < keyj ||
			keyi == keyj && keys[i] < keys[j]
	})

	return keys
}

// sortvals sorts the keys for the top nodes of a tree and returns a slice of the corresponding values.
func sortvals(tr tree) []tree {
	keys := sortkeys(tr)
	vals := make([]tree, len(keys))
	for i, key := range keys {
		vals[i] = tr[key]
	}

	return vals
}

// traverse walks the tree and invokes function fn for each node.
func traverse(tr tree, indent int, fn func(indent int, s string)) {
	for _, u := range sortkeys(tr) {
		v := tr[u]
		fn(indent, u)
		traverse(v, indent+1, fn)
	}
}

// subdir acts like filepath.Rel() but returns an error if the target path is not on the base path.
func subdir(base, targ string) (string, error) {
	if rel, err := filepath.Rel(base, targ); err != nil {
		return "", err
	} else if len(rel) > 1 && rel[:2] == ".." {
		return "", fmt.Errorf("target path %s is not on base path %s", targ, base)
	} else {
		return rel, nil
	}
}
