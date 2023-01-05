package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
)

var (
	// skipdirs identifies directories to ignore for parsing.
	skipdirs = map[string]struct{}{
		"internal": {},
		"testdata": {},
	}

	// fileSet to keep track of all the parsing.
	fileSet = token.NewFileSet()

	parsedDirs = map[string]struct{}{}
)

func parse(dir string) {
	if _, ok := parsedDirs[dir]; ok {
		return
	}
	parsedDirs[dir] = struct{}{}

	for skip := range skipdirs {
		if strings.Contains(dir, skip) {
			return
		}
	}

	pkgs, err := parser.ParseDir(
		fileSet,
		dir,
		func(filter fs.FileInfo) bool {
			return true
		},
		parser.ParseComments, // read comments for go:build constraints
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "=================== Parse Dir:", dir, err)
		return
	}

	fmt.Fprintln(os.Stderr, "=================== Parse Dir:", dir, pkgs)

	for _, pkg := range pkgs {
		if len(pkgs) > 1 && (strings.HasSuffix(pkg.Name, "_test") || pkg.Name == "main") {
			// skip embedded non-API packages
			continue
		}
		ast.Walk(
			visitor{
				pkg: pkg,
			},
			pkg,
		)
	}
}
