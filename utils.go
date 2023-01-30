// Copyright © 2023 The Gomon Project.

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type (
	// t is the generic type for a tree.
	t[T ~string] map[T]t[T]

	// tree is a tree of strings.
	tree = t[string]
)

// add inserts a node into a tree.
func add[T ~string](tree t[T], nodes ...T) {
	if len(nodes) > 0 {
		if _, ok := tree[nodes[0]]; !ok {
			tree[nodes[0]] = t[T]{}
		}
		add(tree[nodes[0]], nodes[1:]...)
	}
}

// sortkeys sorts the keys for the top nodes of a tree and returns a slice of the keys.
func sortkeys[T ~string](tree t[T]) []T {
	keys := make([]T, len(tree))
	i := 0
	for key := range tree {
		if key != "" {
			keys[i] = key
			i++
		}
	}
	keys = keys[0:i]

	sort.Slice(keys, func(i, j int) bool {
		keyi, keyj := keys[i], keys[j]
		keyi = T(strings.Trim(string(keyi), "*()"))
		keyj = T(strings.Trim(string(keyj), "*()"))
		return keyi < keyj ||
			keyi == keyj && keys[i] < keys[j]
	})

	return keys
}

// sortvals sorts the keys for the top nodes of a tree and returns a slice of the corresponding values.
func sortvals[T ~string](tree t[T]) []t[T] {
	keys := sortkeys(tree)
	vals := make([]t[T], len(keys))
	for i, key := range keys {
		vals[i] = tree[key]
	}

	return vals
}

// traverse walks the tree and invokes function fn for each node.
func traverse[T ~string](tree t[T], indent int, fn func(indent int, s T)) {
	for _, u := range sortkeys(tree) {
		v := tree[u]
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
