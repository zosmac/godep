// Copyright © 2021 The Gomon Project.

package main

import (
	"fmt"
	"hash/fnv"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/zosmac/gomon/core"
)

var (
	// the top-level subgraphs.
	standard, module, imports, vendor = "std", gomod, "import", "vendor"

	// dirmap maps the source directory paths to their subgraphs.
	dirmap = map[string]string{
		dirstd:  standard,
		dirimps: imports,
	}

	// subgtmpl is the layout for a graphviz subgraph statement. The %c
	// formatter at the beginning is for a character to facilitate sorting
	// the subgraph and node statements and closing characters '}' for a
	// subgraph and ']' for a node's tooltip. Use \x00 to sort subgraph
	// statements first. Use \x7F to sort closing characters ']' and '}'
	// last. Trim this character when inserting into the nodegraph.
	subgtmpl = "%c\nsubgraph %q { cluster=true fontcolor=black bgcolor=%q label=%q %s"

	// nodetmpl is the layout for a graphviz node statement. The initial
	// pad space character is trimmed from each statement as it is inserted
	// into the graphviz nodegraph.
	nodetmpl = " \n%q [fillcolor=%q label=%q tooltip=\""

	// graphmap maps standard, (module), imports, and vendor packages to the top graphvis subgraphs.
	graphmap = map[string]string{
		standard: fmt.Sprintf(subgtmpl, 0x01, standard, "lightgrey", "Go Standard Packages", "rank=source"),
		imports:  fmt.Sprintf(subgtmpl, 0x03, imports, "lightgrey", "Imported Packages", "rank=sink"),
		vendor:   fmt.Sprintf(subgtmpl, 0x04, vendor, "lightgrey", "Vendored Packages", "rank=sink"),
	}

	// subgmap maps the 'branch' package paths to graphvis subgraph statements.
	subgmap = map[string]string{}

	// nodemap maps the 'leaf' package paths to graphviz node statements.
	nodemap = map[string]string{}

	// nodes contains the graphviz layout of subgraphs and nodes.
	nodes = tree{
		graphmap[standard]: tree{"\x7F\n}": tree{}},
		graphmap[imports]:  tree{"\x7F\n}": tree{}},
		graphmap[vendor]:   tree{"\x7F\n}": tree{}},
	}

	// edges contains all the links between nodes.
	edges = tree{}

	// colors on HSV spectrum that work well in light and dark mode
	colors = []string{
		"0.0 0.5 0.80",
		"0.1 0.5 0.75",
		"0.2 0.5 0.7 ",
		"0.3 0.5 0.75",
		"0.4 0.5 0.75",
		"0.5 0.5 0.75",
		"0.6 0.5 0.9 ",
		"0.7 0.5 1.0", // blue needs to be a bit brighter
		"0.8 0.5 0.9",
		"0.9 0.5 0.85",
	}

	// hash used to compute colors index
	hash = fnv.New64()
)

// color defines the color for graphviz nodes and edges
func color(s string) string {
	hash.Write([]byte(s))
	i := hash.Sum64()
	hash.Reset()
	return colors[i%uint64(len(colors))]
}

// nodegraph produces the package connections node graph.
func nodegraph(references tree) string {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			buf = buf[:n]
			core.LogError(fmt.Errorf("nodegraph() panicked, %v\n%s", r, buf))
		}
	}()

	if dirmod != dirstd {
		dirmap[dirmod] = module
		graphmap[module] = fmt.Sprintf(subgtmpl, 0x02, module, "lightgrey", gomod, "rank=same")
		nodes[graphmap[module]] = tree{"\x7F\n}": tree{}}
	}

	for _, refs := range references {
		for rabs, defs := range refs {
			r, rnode, rtree := node(rabs)

			for dabs := range defs {
				d, dnode, dtree := node(dabs)

				if dnode == rnode || // ignore intra-node calls
					dirmod != dirstd && d != 2 && r != 2 { // neither is in module
					continue
				}

				rtree[" "+rnode+"\\n"] = tree{}
				rtree[" "+dnode+"\\n"] = tree{}
				dtree[" "+rnode+"\\n"] = tree{}
				dtree[" "+dnode+"\\n"] = tree{}

				dir := "back"
				if r < d ||
					r == d && rnode < dnode {
					dir = "forward"
					rnode, dnode = dnode, rnode
				}
				ports := "tailport=e headport=w" // 'e', 'w' ONLY way to ensure edge on correct side
				if d == 1 && r == 1 {
					ports = "tailport=w headport=w" // left side for standard's intra-cluster references
				} else if d == r {
					ports = "tailport=e headport=e" // right side for others' intra-cluster references
				}

				edges[fmt.Sprintf(
					"\n%q -> %q [%s dir=%s color=%q tooltip=\"%[1]s\\n%s\"]",
					dnode,
					rnode,
					ports,
					dir,
					color(rnode)+";0.5:"+color(dnode),
				)] = tree{}
			}
		}
	}

	graph := fmt.Sprintf(`digraph "Module \"%s\" Packages Nodegraph" {
  label="\G %s"
  labelloc=t
  fontname="sans-serif"
  fontsize=14.0
  fontcolor=lightgrey
  bgcolor=black
  rankdir=LR
  newrank=true
  compound=true
  ordering=out
  nodesep=0.05
  ranksep=8
  node [shape=rect style="filled" height=0.3 width=1.5 margin="0.2,0.0" fontname="sans-serif" fontsize=11.0]
  edge [penwidth=2.0]`,
		gomod,
		time.Now().Local().Format("Mon Jan 02 2006 at 03:04:05PM MST"),
	)

	traverse(nodes, 0, func(indent int, s string) {
		graph += s[1:]
	})

	traverse(edges, 0, func(indent int, s string) {
		graph += s
	})

	graph += "\n}\n"

	return graph
}

func node(abs string) (byte, string, tree) {
	for pth, tg := range dirmap {
		pkg, err := subdir(pth, abs) // get package name
		if err != nil {
			continue
		}

		// check if module is an imported package; if so skip resolving to imports directory
		if _, err := subdir(dirmod, abs); err == nil && pth == dirimps {
			continue // ...on to dirmod, which happens to be a subdirectory of dirimps
		}

		// treat vendor directory as vendored package path
		if _, a, ok := strings.Cut(abs, "/"+vendor+"/"); ok {
			tg = vendor
			pkg = a
		}

		gr := graphmap[tg]
		order := gr[0] // first byte corresponds to order of top graph standard, module, imports, vendored

		tr := nodes[gr]

		dirs := strings.Split(path.Dir(pkg), "/")
		base := path.Base(pkg)
		pkg = ""
		for _, dir := range dirs {
			if dir == "." {
				break
			}
			pkg = path.Join(pkg, dir)
			node := tg + ": " + pkg

			// cache dot subgraph statement
			sg, ok := subgmap[node]
			if !ok {
				sg = fmt.Sprintf(subgtmpl, 0x00, pkg, color(pkg), pkg, "rank=same")
				subgmap[node] = sg
			}

			// add dot subgraph statement to node graph
			if _, ok := tr[sg]; !ok {
				tr[sg] = tree{"\x7F\n}": tree{}}
			}

			// if previously added package node (e.g. io) is parent of this
			// node (e.g. io/fs), move it (i.e. io) into this subgraph
			nd := fmt.Sprintf(nodetmpl, node, color(node), pkg)
			if n, ok := tr[nd]; ok {
				delete(tr, nd)
				tr[sg][nd] = n
			}

			tr = tr[sg]
		}

		if pkg = path.Join(pkg, base); pkg == "." {
			pkg = tg // package == module
		}
		node := tg + ": " + pkg

		// if nested node (e.g. io/fs) for this node already
		// exists, place this node (i.e. io) in its subgraph.
		if sg, ok := subgmap[node]; ok {
			if _, ok := tr[sg]; !ok {
				tr[sg] = tree{"\x7F\n}": tree{}}
			}
			tr = tr[sg]
		}

		// cache dot node statement
		nd, ok := nodemap[node]
		if !ok {
			nd = fmt.Sprintf(nodetmpl, node, color(node), pkg)
			nodemap[node] = nd
		}

		// add dot node statement to dot subgraph
		if _, ok := tr[nd]; !ok {
			tr[nd] = tree{"\x7F\"]": tree{}} // close tooltip and node attributes
		}
		tr = tr[nd]

		return order, node, tr
	}

	return 0, "", tree{}
}
