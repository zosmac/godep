package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/types"
	"os"
	"os/exec"
	"path"
	"strings"
)

var (
	// dirstd is the location of the Go standard packages source.
	dirstd = path.Join(build.Default.GOROOT, "src")

	// dirimps is the location of the source for imported Go packages.
	dirimps = path.Join(build.Default.GOPATH, "pkg", "mod")

	// gomod, dirmod are the import path and directory location of the module.
	gomod, dirmod = func() (string, string) {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}\n{{.Dir}}")
		if out, err := cmd.Output(); err == nil {
			if b, a, ok := strings.Cut(string(out), "\n"); ok && b != "" {
				a, _, _ := strings.Cut(a, "@")
				return strings.TrimSpace(b), strings.TrimSpace(a)
			}
		}
		panic(fmt.Sprintf("no go.mod found from current directory %q", cwd))
	}()

	// imps tree reports all imported packages.
	imps = tree{}

	// aliases map selection names used in a file to the imported package names.
	aliases = map[string]string{} // alias:package

	// ifcs tree reports all interfaces.
	ifcs = tree{}

	// typs tree reports all exported types.
	typs = tree{}

	// vals tree reports all exported values.
	vals = tree{}

	// fncs tree reports all exported functions.
	fncs = tree{}

	// defs tree reports where types, values, and functions are defined.
	defs = tree{}

	// refs tree reports where types, values, and functions are referenced.
	refs = tree{}

	// sets tree reports interfaces with types whose method sets comply.
	sets = tree{}
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
		if !ast.IsExported(node.Name.Name) {
			break
		}
		name := v.pkg.Name + "." + node.Name.Name
		if _, ok := defs[name]; !ok {
			defs[name] = tree{}
		}
		defs[name][v.path(node)] = tree{}

		if node.Recv == nil || len(node.Recv.List) == 0 {
			addFnc(v, node)
		} else {
			addMth(v, node)
		}

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
	if _, ok := imps[pkg]; !ok {
		imps[pkg] = tree{}
	}
	imps[pkg][abs] = tree{}
}

// addTyp adds a type to the typs or ifcs list.
func addTyp(v visitor, node *ast.TypeSpec) {
	if !ast.IsExported(node.Name.Name) {
		return
	}
	name := v.pkg.Name + "." + node.Name.Name
	if _, ok := defs[name]; !ok {
		defs[name] = tree{}
	}
	defs[name][v.path(node)] = tree{}

	switch typ := node.Type.(type) {
	case *ast.InterfaceType:
		addIfc(v, node.Name.Name, typ)
	case *ast.StructType:
		addStr(v, node.Name.Name, typ)
	case *ast.CompositeLit:
		addLit(v, node.Name.Name, typ)
	default:
		addOth(v, node.Name.Name, typ)
	}
}

// addVal adds a value to the list of values.
func addVal(v visitor, node *ast.ValueSpec) {
	for _, id := range node.Names {
		if !ast.IsExported(id.Name) {
			continue
		}
		name := v.pkg.Name + "." + id.Name
		if _, ok := defs[name]; !ok {
			defs[name] = tree{}
		}
		defs[name][v.path(node)] = tree{}

		if _, ok := vals[name]; !ok {
			vals[name] = tree{}
		}
		for _, val := range node.Values {
			vals[name][types.ExprString(val)] = tree{}
		}
	}
}

// addFnc adds a function to the list of functions.
func addFnc(v visitor, node *ast.FuncDecl) {
	f := v.pkg.Name + "." + node.Name.Name // functions key off function type
	if _, ok := fncs[f]; !ok {
		fncs[f] = tree{}
	}
	fncs[f][node.Name.Name+signature(node.Type)] = tree{} // signature(node.Type)] = tree{}
}

// addMth adds a method for a type.
func addMth(v visitor, node *ast.FuncDecl) {
	typ := node.Recv.List[0].Type
	if s, ok := typ.(*ast.StarExpr); ok {
		typ = s.X
	}
	name := types.ExprString(typ) // methods key off reciever type
	if !ast.IsExported(name) {
		return
	}
	name = v.pkg.Name + "." + name
	if _, ok := typs[name]; !ok {
		typs[name] = tree{}
	}
	typs[name][node.Name.Name+signature(node.Type)] = tree{}
}

// addRef adds the location where an identifier is referenced.
func addRef(v visitor, qualifier string, node *ast.Ident) {
	if !ast.IsExported(node.Name) {
		return
	}
	if pkg := aliases[qualifier]; pkg != "" {
		ref := pkg + "." + node.Name
		if _, ok := refs[ref]; !ok {
			refs[ref] = tree{}
		}
		refs[ref][v.path(node)] = tree{}
	}
}

// addLit adds the elements of a literal type.
func addLit(v visitor, typ string, expr *ast.CompositeLit) {
	typ = v.pkg.Name + "." + typ
	if _, ok := typs[typ]; !ok {
		typs[typ] = tree{}
	}
	lit := types.ExprString(expr.Type)
	if _, ok := typs[typ][lit]; !ok {
		typs[typ][lit] = tree{}
	}
	for _, elt := range expr.Elts {
		typs[typ][lit][types.ExprString(elt)] = tree{}
	}
}

// addIfc adds an interface and its methods to the list of interfaces.
func addIfc(v visitor, name string, expr *ast.InterfaceType) {
	if _, ok := ifcs[v.pkg.Name+"."+name]; !ok {
		ifcs[v.pkg.Name+"."+name] = tree{}
	}

	for _, mth := range expr.Methods.List {
		if len(mth.Names) == 0 {
			// embedded interface type
			dt := types.ExprString(mth.Type)
			if !strings.Contains(dt, ".") && ast.IsExported(dt) {
				dt = v.pkg.Name + "." + dt // interface is in this package
			}
			ifcs[v.pkg.Name+"."+name][dt] = tree{}
		} else {
			for _, id := range mth.Names {
				ifcs[v.pkg.Name+"."+name][id.Name+signature(mth.Type.(*ast.FuncType))] = tree{}
			}
		}
	}
}

// addStr adds a structure declaration to the list of types.
func addStr(v visitor, typ string, expr *ast.StructType) {
	typ = v.pkg.Name + "." + typ
	if _, ok := typs[typ]; !ok {
		typs[typ] = tree{}
	}
	for _, fld := range expr.Fields.List {
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
			typ := fld.Type
			if s, ok := typ.(*ast.StarExpr); ok {
				typ = s.X
			}
			line += types.ExprString(typ)
		}
		if ast.IsExported(line) {
			typs[typ][line] = tree{}
		}
	}
}

// addOth adds any other type to the list of types.
func addOth(v visitor, typ string, expr ast.Expr) {
	typ = v.pkg.Name + "." + typ
	if _, ok := typs[typ]; !ok {
		typs[typ] = tree{}
	}
	typs[typ][types.ExprString(expr)] = tree{}
}

func signature(fnc *ast.FuncType) (sgn string) {
	var tps []string
	for _, tp := range fnc.Params.List {
		typ := types.ExprString(tp.Type)
		tps = append(tps, typ)
		for i := 1; i < len(tp.Names); i++ {
			tps = append(tps, typ)
		}
	}
	sgn = "(" + strings.Join(tps, ", ") + ")"

	if fnc.Results != nil {
		tps = nil
		for _, tp := range fnc.Results.List {
			typ := types.ExprString(tp.Type)
			tps = append(tps, typ)
			for i := 1; i < len(tp.Names); i++ {
				tps = append(tps, typ)
			}
		}
		if tps != nil {
			if len(tps) == 1 {
				sgn += " " + tps[0]
			} else {
				sgn += " (" + strings.Join(tps, ", ") + ")"
			}
		}
	}

	return
}
