//go:build ignore

package main

import (
	"fmt"
	"go/types"
	"os"
	"path"
	"strings"
)

type (
	importr struct {
	}
)

func (imp *importr) Import(pth string) (*types.Package, error) {
	return nil, fmt.Errorf("this importer does not support Import(path), use ImportFrom()")
}

func (imp *importr) ImportFrom(pth, from string, mode types.ImportMode) (*types.Package, error) {
	for skip := range skipdirs {
		if strings.Contains(pth, skip) {
			return nil, fmt.Errorf("skip import of package %s with %s in path", pth, skip)
		}
	}

	// determine local directory path from import path
	var dir string
	if pth == "C" { // the "C" package?
		return nil, fmt.Errorf("cannot import C package")
	} else if _, err := subdir(gomod, pth); err == nil { // module package?
		dir = path.Join(dirmod, pth)
	} else if _, err := os.Stat(path.Join(dirstd, pth)); err == nil { // std package?
		dir = path.Join(dirstd, pth)
	} else {
		dir = path.Join(dirimps, pth) // default to imported package
	}

	if _, ok := tpkgs[dir]; !ok {
		parse(dir)
	}

	tpkg, ok := tpkgs[dir]
	if !ok {
		return nil, fmt.Errorf("types package %s import returned nil", dir)
	}

	return tpkg, nil
}
