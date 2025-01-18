// Copyright © 2023 The Gomon Project.

package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/types"
	"os"
	"path"
	"strings"

	"github.com/zosmac/gocore"
	"golang.org/x/tools/go/packages"
)

type (
	// visitor employed by the AST walk of the parse function.
	visitor struct {
		pkg *packages.Package
	}

	// tree organizes information types parsed from packages.
	tree = gocore.Tree[string]

	// meta describes the content and ordering of the tree.
	meta = gocore.Meta[string, any, string]

	// TREE is the enumberation type for information types parsed from packages.
	TREE int
)

// enumeration of each tree type.
const (
	IMPORTS TREE = iota
	INTERFACES
	TYPES
	VALUES
	FUNCTIONS
	DEFINES
	REFERENCES
	IMPLEMENTS
	TREES
)

var (
	// names of each tree type.
	names = map[TREE]string{
		IMPORTS:    "IMPORTS",    // imps tree reports all imported packages.
		INTERFACES: "INTERFACES", // ifcs tree reports all interfaces.
		TYPES:      "TYPES",      // typs tree reports all exported types.
		VALUES:     "VALUES",     // vals tree reports all exported values.
		FUNCTIONS:  "FUNCTIONS",  // fncs tree reports all exported functions.
		DEFINES:    "DEFINES",    // defs tree reports where types, values, and functions are defined.
		REFERENCES: "REFERENCES", // refs tree reports where types, values, and functions are referenced.
		IMPLEMENTS: "IMPLEMENTS", // sets tree reports interfaces with types whose method sets comply.
	}

	// dirstd is the location of the Go standard packages source.
	dirstd = path.Join(build.Default.GOROOT, "src")

	// dirimps is the location of the source for imported Go packages.
	dirimps = path.Join(build.Default.GOPATH, "pkg", "mod")

	// gomod, dirmod are the import path and directory location of the module.
	gomod, dirmod string

	// aliases map selection names used in a file to the imported package names.
	aliases = map[string]string{} // alias:package

	// trees creates a slice that anchors all of the information types parsed from packages.
	trees = func() []tree {
		ts := make([]tree, TREES)
		for i := range ts {
			ts[i] = tree{}
		}
		return ts
	}()
)

// path determines the location of a node.
func (v visitor) path(node ast.Node) string {
	pth := fileSet.File(node.Pos()).Name()
	if b, a, ok := strings.Cut(pth, "@"); ok { // strip version
		if _, a, ok := strings.Cut(a, "/"); ok { // reassemble path
			pth = path.Join(b, a)
		} else {
			pth = b
		}
	}
	if ext := path.Ext(pth); ext == ".go" {
		pth = path.Dir(pth)
	}
	return pth
}

// Visit evaluates each node of the AST.
func (v visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch node := node.(type) {

	// STATEMENTS
	case *ast.AssignStmt,
		*ast.BadStmt,
		*ast.BlockStmt,
		*ast.BranchStmt,
		*ast.CaseClause,
		*ast.CommClause,
		*ast.DeclStmt,
		*ast.DeferStmt,
		*ast.EmptyStmt,
		*ast.ExprStmt,
		*ast.ForStmt,
		*ast.GoStmt,
		*ast.IfStmt,
		*ast.IncDecStmt,
		*ast.LabeledStmt,
		*ast.RangeStmt,
		*ast.ReturnStmt,
		*ast.SelectStmt,
		*ast.SendStmt,
		*ast.SwitchStmt,
		*ast.TypeSwitchStmt:

	case ast.Stmt: // put this last after all the explicit statement types
		panic(fmt.Errorf("unexpected stmt type %T %[1]s", node))

	// IDENTITY EXPRESSION
	case *ast.Ident:
		v.addRef(v.pkg.Name, node)

	// LITERAL EXPRESSIONS
	case *ast.BasicLit,
		*ast.CompositeLit,
		*ast.Ellipsis,
		*ast.FuncLit:

	// TYPE EXPRESSIONS
	case *ast.ArrayType,
		*ast.ChanType,
		*ast.FuncType,
		*ast.InterfaceType,
		*ast.MapType,
		*ast.StructType:

	// COMPLEX EXPRESSIONS
	case *ast.BinaryExpr,
		*ast.CallExpr,
		*ast.IndexExpr,
		*ast.IndexListExpr,
		*ast.KeyValueExpr,
		*ast.ParenExpr,
		*ast.SliceExpr,
		*ast.StarExpr,
		*ast.TypeAssertExpr,
		*ast.UnaryExpr:

	case *ast.SelectorExpr:
		v.addRef(types.ExprString(node.X), node.Sel)

	case ast.Expr: // put this last after all the explicit expression types
		panic(fmt.Errorf("unexpected expr type %T %[1]s", node))

	// SPECS
	case *ast.ImportSpec:
		for skip := range skipdirs {
			if strings.Contains(node.Path.Value, skip) {
				return nil
			}
		}
		addImp(node)

	case *ast.TypeSpec:
		v.addTyp(node)

	case *ast.ValueSpec:
		v.addVal(node)

	case ast.Spec:
		panic(fmt.Errorf("unexpected spec type %T %[1]s", node))

	// NODES
	case *ast.Package:
		for pth, file := range node.Files {
			if !gobuild(pth, file) {
				delete(node.Files, pth)
			}
		}

	case *ast.File:
		aliases = map[string]string{}

	case *ast.FuncDecl:
		v.addFnc(node)

	case *ast.CommentGroup,
		*ast.Comment,
		*ast.FieldList,
		*ast.Field,
		*ast.GenDecl:

	default:
		panic(fmt.Errorf("unexpected node type %T %[1]s", node))
	}

	return v
}

// gobuild evaluates a file's build constraints to determine whether to parse it.
func gobuild(pth string, file *ast.File) bool {
	for _, group := range file.Comments { // look for go:build
		if group.Pos() > file.Package {
			break // skip comments after the package statement
		}
		for _, comment := range group.List {
			if constraint.IsGoBuild(comment.Text) {
				expr, _ := constraint.Parse(comment.Text)
				return expr.Eval(func(tag string) bool {
					return tag == build.Default.GOOS
				})
			}
		}
	}

	if pth == "" {
		return true
	}

	// create build constraint from file name
	if s := strings.Join( // build constraint expression
		strings.Split(
			strings.TrimSuffix( // remove file extension
				path.Base(pth),
				path.Ext(pth),
			),
			"_")[1:], // separate name from build constraints
		" && ",
	); len(s) == 0 { // no constraints in file name
		return true
	} else { // evaluate constraints in file name
		expr, _ := constraint.Parse("//go:build " + s)
		return expr.Eval(func(tag string) bool {
			return tag == build.Default.GOOS
		})
	}
}

// addImp adds an import to the list of imports.
func addImp(node *ast.ImportSpec) {
	pth := strings.Trim(node.Path.Value, "\"")
	pkg, _, _ := strings.Cut(path.Base(pth), ".") // if package name has ".", strip following (i.e. version)

	if pth == "C" { // skip "C" package
		return
	}

	// convert import path to local directory path
	var abs string
	if rel, err := gocore.Subdir(dirmod, pth); err == nil { // package in current module
		abs = path.Join(dirmod, rel)
	} else if _, err := os.Stat(path.Join(dirstd, pth)); err == nil { // std package
		abs = path.Join(dirstd, pth)
	} else {
		abs = path.Join(dirimps, pth) // package from imports
	}

	var alias string
	if node.Name == nil {
		alias = pkg
	} else {
		alias = node.Name.Name
	}
	aliases[alias] = pkg
	trees[IMPORTS].Add(pkg, abs)
}

// addTyp adds a type to the trees[TYPES] or trees[INTERFACES] list.
func (v visitor) addTyp(node *ast.TypeSpec) {
	if !ast.IsExported(node.Name.Name) {
		return
	}
	v.addDef(node.Name)

	name := v.pkg.Name + "." + node.Name.Name
	switch expr := node.Type.(type) {
	case *ast.InterfaceType:
		v.addIfc(name, expr)
	case *ast.StructType:
		addStr(name, expr)
	case *ast.CompositeLit:
		lit := types.ExprString(expr.Type)
		for _, elt := range expr.Elts {
			trees[TYPES].Add(name, lit, types.ExprString(elt))
		}
	default:
		trees[TYPES].Add(name, types.ExprString(expr))
	}
}

// addIfc adds an interface and its methods to the list of interfaces.
func (v visitor) addIfc(name string, node *ast.InterfaceType) {
	for _, mth := range node.Methods.List {
		if len(mth.Names) == 0 {
			// embedded interface type
			dt := types.ExprString(mth.Type)
			if !strings.Contains(dt, ".") && ast.IsExported(dt) {
				dt = v.pkg.Name + "." + dt // interface is in this package
			}
			trees[INTERFACES].Add(name, dt)
		} else {
			for _, id := range mth.Names {
				trees[INTERFACES].Add(name, id.Name+signature(mth.Type.(*ast.FuncType)))
			}
		}
	}
}

// addStr adds a structure declaration to the list of types.
func addStr(name string, node *ast.StructType) {
	for _, fld := range node.Fields.List {
		names := make([]string, len(fld.Names))
		for i, id := range fld.Names {
			names[i] = id.Name
		}
		line := strings.Join(names, ", ")
		if fnc, ok := fld.Type.(*ast.FuncType); ok {
			line += signature(fnc)
		} else {
			if len(line) > 0 {
				line += " "
			}
			expr := fld.Type
			if s, ok := expr.(*ast.StarExpr); ok {
				expr = s.X
			}
			line += types.ExprString(expr)
		}
		if ast.IsExported(line) {
			trees[TYPES].Add(name, line)
		}
	}
}

// addVal adds a value to the list of values.
func (v visitor) addVal(node *ast.ValueSpec) {
	for _, id := range node.Names {
		if !ast.IsExported(id.Name) {
			continue
		}
		v.addDef(id)

		name := v.pkg.Name + "." + id.Name
		for _, val := range node.Values {
			trees[VALUES].Add(name, types.ExprString(val))
		}
	}
}

// addFnc adds function to the functions list or method to a type in the types list
func (v visitor) addFnc(node *ast.FuncDecl) {
	if !ast.IsExported(node.Name.Name) {
		return
	}
	v.addDef(node.Name)

	if node.Recv == nil || len(node.Recv.List) == 0 {
		trees[FUNCTIONS].Add(v.pkg.Name + "." + node.Name.Name + signature(node.Type))
	} else {
		expr := node.Recv.List[0].Type
		if s, ok := expr.(*ast.StarExpr); ok {
			expr = s.X
		}
		name := types.ExprString(expr) // methods key off receiver type
		if !ast.IsExported(name) {
			return
		}
		trees[TYPES].Add(v.pkg.Name+"."+name, node.Name.Name+signature(node.Type))
	}
}

// addDef adds the location where an identifier is defined.
func (v visitor) addDef(id *ast.Ident) {
	trees[DEFINES].Add(v.pkg.Name+"."+id.Name, v.pkg.Dir)
}

// addRef adds the location where an identifier is referenced.
func (v visitor) addRef(qualifier string, id *ast.Ident) {
	if !ast.IsExported(id.Name) {
		return
	}
	if pkg := aliases[qualifier]; pkg != "" {
		trees[REFERENCES].Add(pkg+"."+id.Name, v.pkg.Dir)
	}
}

// signature formats the parameter and result types of a function or method.
func signature(node *ast.FuncType) string {
	parms := "(" + typelist(node.Params) + ")"
	rslts := typelist(node.Results)
	if rslts != "" {
		parms += " "
	}
	if strings.Contains(rslts, ",") {
		rslts = " (" + rslts + ")"
	}

	return parms + rslts
}

// typelist reports the types of a list of fields.
func typelist(flds *ast.FieldList) string {
	if flds == nil {
		return ""
	}
	var typs []string
	for _, fld := range flds.List {
		typ := types.ExprString(fld.Type)
		typs = append(typs, typ)
		for range len(fld.Names) {
			typs = append(typs, typ)
		}
	}

	return strings.Join(typs, ", ")
}
