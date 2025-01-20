// Copyright © 2023 The Gomon Project.

package main

import (
	"fmt"
	"hash/fnv"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/zosmac/gocore"
)

var (
	// the top-level subgraphs.
	standard, imports = "std", "import"

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

	// graphmap maps standard, (module), and imports/vendor packages to the top graphvis subgraphs.
	graphmap = map[string]string{
		standard: fmt.Sprintf(subgtmpl, 0x01, standard, "lightgrey", "Go Standard Packages",
			"rank=same\n\"Standard Packages\" [color=white fillcolor=white fontcolor=black]"),
		imports: fmt.Sprintf(subgtmpl, 0x03, imports, "lightgrey", "Imported/Vendored Packages",
			"rank=same\n\"Imported Packages\" [color=white fillcolor=white fontcolor=black]"),
	}

	// subgmap maps the 'branch' package paths to graphvis subgraph statements.
	subgmap = map[string]string{}

	// nodemap maps the 'leaf' package paths to graphviz node statements.
	nodemap = map[string]string{}

	// nodes contains the graphviz layout of subgraphs and nodes.
	nodes = tree{
		graphmap[standard]: tree{"\x7F\n}": tree{}},
		graphmap[imports]:  tree{"\x7F\n}": tree{}},
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
			gocore.Error("nodegraph", fmt.Errorf("%v", r), map[string]string{
				"stacktrace": string(buf),
			}).Err()
		}
	}()

	if dirmod != dirstd {
		dirmap[dirmod] = gomod
		graphmap[gomod] = fmt.Sprintf(subgtmpl, 0x02, gomod, "lightgrey", gomod,
			"rank=same\n\""+gomod+"\" [color=white fillcolor=white fontcolor=black]")
		nodes[graphmap[gomod]] = tree{"\x7F\n}": tree{}}
	}

	for _, refs := range references {
		for rabs, defs := range refs {
			r, rnode, rtree := node(rabs)

			for dabs := range defs {
				d, dnode, dtree := node(dabs)

				if d == r && dnode == rnode || // ignore intra-node calls
					dirmod != dirstd && d != 2 && r != 2 { // neither is in module
					continue
				}

				rtree[" "+rnode+"\\n"] = tree{}
				rtree[" "+dnode+"\\n"] = tree{}
				dtree[" "+rnode+"\\n"] = tree{}
				dtree[" "+dnode+"\\n"] = tree{}

				dir := "back"
				tport, hport := "e", "w" // 'e', 'w' ONLY way to ensure edge on correct side
				if d < r {
				} else if d > r {
					dir = "forward"
					tport, hport = "w", "e"
				} else if d == 1 {
					tport, hport = "w", "w"
				} else {
					tport, hport = "e", "e"
				}

				edges[fmt.Sprintf(
					"\n%q -> %q [dir=%s tailport=%s headport=%s color=%q tooltip=\"%[1]s\\n%s\"]",
					dnode,
					rnode,
					dir,
					tport,
					hport,
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

	for _, s := range (meta{Tree: nodes}).All() {
		graph += s[1:]
	}

	if dirmod == dirstd {
		graph += "\"Standard Packages\" -> \"Imported Packages\" [style=invis ltail=1 lhead=3]\n"
	} else {
		graph += "\"Standard Packages\" -> \"" + gomod + "\" [style=invis ltail=1 lhead=2]\n"
		graph += "\"" + gomod + "\" -> \"Imported Packages\" [style=invis ltail=2 lhead=3]\n"
	}

	for _, s := range (meta{Tree: edges}).All() {
		graph += s
	}

	graph += "\n}\n"

	return graph
}

func node(abs string) (byte, string, tree) {
	for pth, tg := range dirmap {
		pkg, err := gocore.Subdir(pth, abs) // get package name
		if err != nil {
			continue
		}

		// check if module is an imported package; if so skip resolving to imports directory
		if _, err := gocore.Subdir(dirmod, abs); err == nil && pth == dirimps {
			continue // ...on to dirmod, which happens to be a subdirectory of dirimps
		}

		if _, a, ok := strings.Cut(abs, "/vendor/"); ok { // treat content of vendor as import
			tg = imports
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
			pkg = tg // package = module
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
