// Copyright © 2023 The Gomon Project.

package main

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/packages"
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

	// scannedPkgs records that a package has been scanned.
	scannedPkgs = map[string]struct{}{}
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

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  dir,
		Fset: fileSet,
	})
	if err != nil {
		return
	}

	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.Name, "_test") || len(pkgs) > 1 && pkg.Name == "main" {
			// skip embedded non-API packages
			continue
		}

		scan(pkg)

		for _, pkg := range pkg.Imports {
			scan(pkg)
		}
	}
}

func scan(pkg *packages.Package) {
	if _, ok := scannedPkgs[pkg.Dir]; ok {
		return
	}
	scannedPkgs[pkg.Dir] = struct{}{}
	for _, s := range pkg.Syntax {
		ast.Walk(
			visitor{
				pkg: pkg,
			},
			s,
		)
	}
}
