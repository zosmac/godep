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

	"golang.org/x/tools/go/packages"
)

type TREE int

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
	trees = func() []tree {
		ts := make([]tree, TREES)
		for i := range ts {
			ts[i] = tree{}
		}
		return ts
	}()

	// dirstd is the location of the Go standard packages source.
	dirstd = path.Join(build.Default.GOROOT, "src")

	// dirimps is the location of the source for imported Go packages.
	dirimps = path.Join(build.Default.GOPATH, "pkg", "mod")

	// gomod, dirmod are the import path and directory location of the module.
	gomod, dirmod = func() (string, string) {
		pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedModule, Dir: cwd})
		module := pkgs[0].Module
		mod := module.Path
		dir := module.Dir
		if err != nil || mod == "" || dir == "" {
			panic(fmt.Sprintf("go.mod not resolved %q, %v", dir, err))
		}
		return mod, dir
	}()

	// imps tree reports all imported packages.
	imps = trees[IMPORTS]

	// aliases map selection names used in a file to the imported package names.
	aliases = map[string]string{} // alias:package

	// ifcs tree reports all interfaces.
	ifcs = trees[INTERFACES]

	// typs tree reports all exported types.
	typs = trees[TYPES]

	// vals tree reports all exported values.
	vals = trees[VALUES]

	// fncs tree reports all exported functions.
	fncs = trees[FUNCTIONS]

	// defs tree reports where types, values, and functions are defined.
	defs = trees[DEFINES]

	// refs tree reports where types, values, and functions are referenced.
	refs = trees[REFERENCES]

	// sets tree reports interfaces with types whose method sets comply.
	sets = trees[IMPLEMENTS]
)

// visitor employed by the AST walk of the parse function.
type visitor struct {
	pkg *ast.Package
}

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
		panic(fmt.Sprintf("unexpected stmt type %T %[1]s", node))

		// EXPRESSIONS

		// IDENTITY EXPRESSION

	case *ast.Ident:
		addRef(v, v.pkg.Name, node)

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
		addRef(v, types.ExprString(node.X), node.Sel)

	case ast.Expr: // put this last after all the explicit expression types
		panic(fmt.Sprintf("unexpected expr type %T %[1]s", node))

		// SPECS

	case *ast.ImportSpec:
		for skip := range skipdirs {
			if strings.Contains(node.Path.Value, skip) {
				return nil
			}
		}

		addImp(node)

	case *ast.TypeSpec:
		addTyp(v, node)

	case *ast.ValueSpec:
		addVal(v, node)

	case ast.Spec:
		panic(fmt.Sprintf("unexpected spec type %T %[1]s", node))

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
		addFnc(v, node)

	case *ast.CommentGroup,
		*ast.Comment,
		*ast.FieldList,
		*ast.Field,
		*ast.GenDecl:

	default:
		panic(fmt.Sprintf("unexpected node type %T %[1]s", node))
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
	if rel, err := subdir(gomod, pth); err == nil { // package in current module
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
	add(imps, pkg, abs)
}

// addTyp adds a type to the typs or ifcs list.
func addTyp(v visitor, node *ast.TypeSpec) {
	if !ast.IsExported(node.Name.Name) {
		return
	}
	addDef(v, node.Name)

	name := v.pkg.Name + "." + node.Name.Name
	switch expr := node.Type.(type) {
	case *ast.InterfaceType:
		addIfc(v, name, expr)
	case *ast.StructType:
		addStr(name, expr)
	case *ast.CompositeLit:
		lit := types.ExprString(expr.Type)
		for _, elt := range expr.Elts {
			add(typs, name, lit, types.ExprString(elt))
		}
	default:
		add(typs, name, types.ExprString(expr))
	}
}

// addIfc adds an interface and its methods to the list of interfaces.
func addIfc(v visitor, name string, node *ast.InterfaceType) {
	for _, mth := range node.Methods.List {
		if len(mth.Names) == 0 {
			// embedded interface type
			dt := types.ExprString(mth.Type)
			if !strings.Contains(dt, ".") && ast.IsExported(dt) {
				dt = v.pkg.Name + "." + dt // interface is in this package
			}
			add(ifcs, name, dt)
		} else {
			for _, id := range mth.Names {
				add(ifcs, name, id.Name+signature(mth.Type.(*ast.FuncType)))
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
			add(typs, name, line)
		}
	}
}

// addVal adds a value to the list of values.
func addVal(v visitor, node *ast.ValueSpec) {
	for _, id := range node.Names {
		if !ast.IsExported(id.Name) {
			continue
		}
		addDef(v, id)

		name := v.pkg.Name + "." + id.Name
		for _, val := range node.Values {
			add(vals, name, types.ExprString(val))
		}
	}
}

// addFnc adds function to the functions list or method to a type in the types list
func addFnc(v visitor, node *ast.FuncDecl) {
	if !ast.IsExported(node.Name.Name) {
		return
	}
	addDef(v, node.Name)

	if node.Recv == nil || len(node.Recv.List) == 0 {
		add(fncs, v.pkg.Name+"."+node.Name.Name+signature(node.Type))
	} else {
		expr := node.Recv.List[0].Type
		if s, ok := expr.(*ast.StarExpr); ok {
			expr = s.X
		}
		name := types.ExprString(expr) // methods key off reciever type
		if !ast.IsExported(name) {
			return
		}
		add(typs, v.pkg.Name+"."+name, node.Name.Name+signature(node.Type))
	}
}

// addDef adds the location where an identifier is defined.
func addDef(v visitor, id *ast.Ident) {
	add(defs, v.pkg.Name+"."+id.Name, v.path(id))
}

// addRef adds the location where an identifier is referenced.
func addRef(v visitor, qualifier string, id *ast.Ident) {
	if !ast.IsExported(id.Name) {
		return
	}
	if pkg := aliases[qualifier]; pkg != "" {
		add(refs, pkg+"."+id.Name, v.path(id))
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
		for i := 1; i < len(fld.Names); i++ {
			typs = append(typs, typ)
		}
	}

	return strings.Join(typs, ", ")
}
