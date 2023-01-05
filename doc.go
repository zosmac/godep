package main

/*
Package gonode defines the gonode command that examines the source files of a Go language
module. It produces a node graph of the relationships of the module's packages with packages
of the Go standard library, of imports, and of those that are vendored. Gonode uses the
Graphvis dot command to produce the nodegraph.
*/
