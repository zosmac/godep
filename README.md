# Welcome to *Gonode*: *Go* module dependency *node* graphing.

- [Welcome to *Gonode*: *Go* module dependency *node* graphing](#welcome-to-gonode-go-module-dependency-node-graphing)
- [Overview](#overview)
- [Installing *Gonode*](#installing-gonode)
- [Using *Gonode*](#using-gonode)

# Overview

The `gonode` command parses the Go module in the current directory using the standard library's `go/ast` package. It builds a package dependency graph for the *[Graphviz]*(https://graphviz.org) `dot` command and produces a compressed SVG file for display.

# Installing *Gonode*

The `gonode` command depends on *Graphviz*. To download and install *[Graphviz]*(https://graphviz.org/download/source/), select a stable release, download its tar file, and build and install it. (Note that `gonode` specifies `-Tsvgz` to the `dot` command. Ensure that the zlib development library is installed on your system, e.g. on Ubuntu `sudo apt install zlib1g-dev`, on Fedora `sudo yum install zlib devel`)
```zsh
tar xzvf =(curl -L "https://gitlab.com/api/v4/projects/4207231/packages/generic/graphviz-releases/7.0.6/graphviz-7.0.6.tar.gz")
cd graphviz-7.0.6
./configure
make
sudo make install
```
With the `dot` command in place, download and install *Gonode*:
```zsh
go install github.com/zosmac/gonode@latest
```

# Using *Gonode*

The `gonode` command takes no arguments. Set the current directory to that for a Go language module, defined by a `go.mod` file. Direct the standard output to a compressed SVG file and open in a browser.
```zsh
() {
  gonode | gunzip -c >$1
  mv $1 $1.svg
  open $1.svg
  sleep 1
  rm $1.svg
} `mktemp /tmp/XXXXXX`
```

<img src="assets/gomon.svg">