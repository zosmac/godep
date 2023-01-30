// Copyright © 2023 The Gomon Project.

package main

import (
	"github.com/zosmac/gocore"
)

// init initializes the command line flags.
func init() {
	gocore.Flags.CommandDescription = `The godep command roduces a Go package dependency graph for the current module.`
}
