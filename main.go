// Copyright © 2023 The Gomon Project.

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zosmac/gocore"
)

var (
	// cwd current working directory with module source.
	cwd, _ = os.Getwd()
)

// display the tree node based on recursion depth.
func display(depth int, node string, _ any) {
	fmt.Fprintf(os.Stderr, "%s%s\n", strings.Repeat("\t", depth), node)
}

// main
func main() {
	gocore.Main(Main)
}

// Main called from gocore.Main.
func Main(ctx context.Context) error {
	if cwd == dirstd {
		gomod, dirmod = standard, dirstd
	} else {
		module := gocore.Module(cwd)
		if module.Dir == "" {
			return gocore.Error("go.mod", errors.New("unresolved"), map[string]string{
				"directory": cwd,
			})
		}
		gomod = module.Path
		dirmod = module.Dir
	}

	if err := walk(cwd); err != nil {
		return gocore.Error("WalkDir", err, map[string]string{
			"directory": cwd,
		})
	}

	defs4refs()

	typesets()

	report()

	os.Stdout.Write(dot(nodegraph(trees[REFERENCES])))

	return nil
}

// walk the directory tree and parse the go files.
func walk(pth string) error {
	if _, err := gocore.Subdir(dirimps, pth); err == nil {
		pth = verspath(pth) // imports include version in path
	}

	return filepath.WalkDir(
		pth,
		func(dir string, entry fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("error walking %q at %s: %w", pth, dir, err)
			}
			if entry.IsDir() {
				base := path.Base(entry.Name())
				if _, ok := skipdirs[base]; ok || base[0] == '.' {
					return filepath.SkipDir
				}
				parse(dir)
			}
			return nil
		},
	)
}

// verspath checks if import path references a versioned name (i.e. @vn.n.n)
func verspath(pth string) string {
	var rem string
	for {
		dir := path.Dir(pth)
		base := path.Base(pth)
		if dir == dirimps {
			return ""
		}
		if ents, err := os.ReadDir(dir); err == nil {
			var vers []string
			for _, ent := range ents {
				if b, a, ok := strings.Cut(ent.Name(), "@"); ok && b == base {
					vers = append(vers, a) // versioned directories for package
				}
			}
			slices.Sort(vers)
			if len(vers) > 0 { // grab latest version
				return path.Join(dir, base+"@"+vers[len(vers)-1], rem)
			}
		}
		pth = dir                  // check the next level up
		rem = path.Join(base, rem) // keep remaining subdirectories together
	}
}

// defs4refs adds the definition location for each referenced type, value, or function.
func defs4refs() {
	for ref, abss := range trees[REFERENCES] {
		for abs := range abss { // check if reference is from module
			if _, err := gocore.Subdir(dirmod, abs); err != nil {
				delete(abss, abs) // remove reference
			}
		}
		if len(abss) == 0 { // skip references only within std and imports
			delete(trees[REFERENCES], ref)
			continue
		}
		if _, ok := trees[DEFINES][ref]; ok { // check if definition is in the current module
			for def := range trees[DEFINES][ref] {
				for abs := range abss {
					abss[abs][def] = tree{}
				}
			}
		} else { // add definition for standard or imported package type
			pkg, _, _ := strings.Cut(ref, ".")
			for imp := range trees[IMPORTS][pkg] {
				if _, err := gocore.Subdir(dirmod, imp); err != nil {
					for abs := range abss {
						abss[abs][imp] = tree{}
					}
				}
			}
		}
		trees[REFERENCES][ref] = abss
	}
}

// typesets finds the interfaces that types implement.
func typesets() {
	// expand embedded interfaces with their methods
	for ifc, mths := range trees[INTERFACES] {
		for mth := range mths {
			if !strings.Contains(mth, "(") {
				// embedded interface, replace with its methods
				delete(mths, mth)
				for m := range trees[INTERFACES][mth] {
					trees[INTERFACES][ifc][m] = tree{}
				}
			}
		}
	}

	// for each type, check if it implements the methods of an interface
	for typ, flds := range trees[TYPES] {
		for ifc, mths := range trees[INTERFACES] {
			i := 0
			for mth := range mths {
				if _, ok := flds[mth]; !ok {
					break
				}
				i++
			}
			if i == len(mths) {
				trees[IMPLEMENTS].Add(ifc, typ)
			}
		}
	}
}

// report echos out all of the trees to stderr.
func report() {
	for i := range len(trees) {
		fmt.Fprintf(os.Stderr, "==== %s ====\n", names[TREE(i)])
		for depth, node := range (meta{Tree: trees[i]}).All() {
			display(depth, node, nil)
		}
	}
}

// dot calls the Graphviz dot command to render the package dependencies as SVG.
func dot(graphviz string) []byte {
	cmd := exec.Command("dot", "-v", "-Tsvg")
	cmd.Stdin = bytes.NewBufferString(graphviz)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		gocore.Error("dot", err, map[string]string{
			"stderr": stderr.String(),
		}).Err()
		sc := bufio.NewScanner(strings.NewReader(graphviz))
		for i := 1; sc.Scan(); i++ {
			fmt.Fprintf(os.Stderr, "%4.d %s\n", i, sc.Text())
		}
		return nil
	}

	return stdout.Bytes()
}
