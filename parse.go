// Copyright © 2023 The Gomon Project.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
)

var (
	// skipdirs identifies directories to ignore for parsing.
	skipdirs = map[string]struct{}{
		"internal": {},
		"testdata": {},
	}

	// fileSet keeps track of all the parsing.
	fileSet = token.NewFileSet()

	// parseDirs records that a directory has been parsed.
	parsedDirs = map[string]struct{}{}
)

// parse invokes the go parser and walks the AST.
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
		return
	}

	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.Name, "_test") || len(pkgs) > 1 && pkg.Name == "main" {
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
