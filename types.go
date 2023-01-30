// Copyright © 2023 The Gomon Project.

//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
)

var (
	tpkgs = map[string]*types.Package{}
)

func typesPackage(pkg *ast.Package) (*types.Package, *types.Info, error) {
	for name, imp := range pkg.Imports {
		fmt.Fprintf(os.Stderr, "NAME %s IMPORT %v\n", name, imp)
	}

	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Instances:  map[*ast.Ident]types.Instance{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Implicits:  map[ast.Node]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Scopes:     map[ast.Node]*types.Scope{},
		InitOrder:  []*types.Initializer{},
	}

	files := make([]*ast.File, len(pkg.Files))
	i := 0
	for _, file := range pkg.Files {
		files[i] = file
		i++
	}
	config := &types.Config{
		// Importer: importer.Default(),
		// Importer: importer.ForCompiler(fileSet, "gc", lookup),
		Importer: &importr{},
	}

	fmt.Fprintln(os.Stderr, "================= config.Check", pkg.Name)
	tpkg, err := config.Check(pkg.Name, fileSet, files, info)
	if err != nil {
		return nil, nil, fmt.Errorf("config.Check for package %s error %v", pkg.Name, err)
	}

	return tpkg, info, nil
}

func typesReport(tpkg *types.Package, info *types.Info) {
	fmt.Fprintf(os.Stderr, "Package: %s\n", tpkg.Path())
	fmt.Fprintf(os.Stderr, "Name:    %s\n", tpkg.Name())
	for _, imp := range tpkg.Imports() {
		fmt.Fprintf(os.Stderr, "Import: %s\n", imp.String())
	}
	tpkg.Scope().WriteTo(os.Stderr, 0, true)

	fmt.Fprintln(os.Stderr, ">>>> Types")
	for expr, typeandvalue := range info.Types {
		fmt.Fprintf(os.Stderr, "\t%T %s\n\t\t%s (%s)\n",
			expr,
			types.ExprString(expr),
			typeandvalue.Type.String(),
			typeandvalue.Type.Underlying().String())
	}

	fmt.Fprintln(os.Stderr, ">>>> Instances")
	for id, instance := range info.Instances {
		switch typ := instance.Type.(type) {
		case *types.Named:
			fmt.Fprintf(os.Stderr, "\tNAMED %s: %s\n", id.Name, typ.String())
		case *types.Signature:
			fmt.Fprintf(os.Stderr, "\tSIGNATURE %s: %s\n", id.Name, typ.String())
		default:
			fmt.Fprintf(os.Stderr, "\t???? %s: %s\n", id.Name, instance.Type.String())
		}
	}

	fmt.Fprintln(os.Stderr, ">>>> Defs")
	for id, obj := range info.Defs {
		if id != nil && obj != nil {
			fmt.Fprintf(os.Stderr, "\t%s: %s\n", id.Name, obj.String())
		}
	}

	fmt.Fprintln(os.Stderr, ">>>> Uses")
	for id, obj := range info.Uses {
		if id != nil && obj != nil {
			fmt.Fprintf(os.Stderr, "\t%s: %s\n", id.Name, obj.String())
		}
	}

	fmt.Fprintln(os.Stderr, ">>>> Implicits")
	for node, implicit := range info.Implicits {
		switch node := node.(type) {
		case *ast.ImportSpec:
			implicit := implicit.(*types.PkgName)
			fmt.Fprintf(os.Stderr, "\t%s %s %s\n",
				implicit.Name(), implicit.Imported().Name(), implicit.Imported().Path())
		case *ast.CaseClause:
			fmt.Fprintf(os.Stderr, "\t%#v %#v\n\t\t%#v\n", node.List, node.Body, implicit)
		case *ast.Field:
			fmt.Fprintf(os.Stderr, "\t%s %#v\n\t\t%#v", types.ExprString(node.Type), node.Names, implicit)
		default:
			fmt.Fprintf(os.Stderr, "\t?????? %T %+[1]v\n\t\t%T %+[2]v\n", node, implicit)
		}
	}

	fmt.Fprintln(os.Stderr, ">>>> Selections")
	for node, selection := range info.Selections {
		fmt.Fprintf(os.Stderr, "\t%T %+[1]v\n\t\t%T %+[2]v\n", node, selection)
	}

	// fmt.Fprintln(os.Stderr, ">>>> Package Scopes")
	// scope := tpkg.Scope()
	// for {
	// 	if strings.Contains(scope.Parent().String(), "universe") {
	// 		break
	// 	}
	// 	if scope = scope.Parent(); scope == nil {
	// 		break
	// 	}
	// }
	// scope.WriteTo(os.Stderr, 0, true)
	// fmt.Fprintln(os.Stderr, ">>>> Info Scopes")
	// for _, scope := range info.Scopes {
	// 	for {
	// 		if strings.Contains(scope.Parent().String(), "universe") {
	// 			break
	// 		}
	// 		if scope = scope.Parent(); scope == nil {
	// 			break
	// 		}
	// 	}
	// 	scope.WriteTo(os.Stderr, 0, true)
	// }

	fmt.Fprintln(os.Stderr, ">>>> InitOrder")
	for _, initorder := range info.InitOrder {
		fmt.Fprintf(os.Stderr, "\t%T %+[1]v\n", initorder)
	}
}

func eval(tpkg *types.Package, info *types.Info) {
	fmt.Fprintf(os.Stderr, "%s\n", tpkg.Name())
	for _, tpkg := range tpkg.Imports() {
		eval(tpkg, info)
	}
}

func object(obj types.Object) string {
	if obj == nil || !obj.Exported() {
		return ""
	}
	var pkg, typ, ult string
	if obj.Pkg() != nil {
		pkg = obj.Pkg().Name()
	}
	if obj.Type() != nil {
		typ = obj.Type().String()
		ult = obj.Type().Underlying().String()
	}
	var line string
	switch obj := obj.(type) {
	case *types.Func:
		line = "FUNCTION: " + pkg + " " + obj.Name() + " " + typ + " " + ult
	case *types.Var:
		line = "VARIABLE: " + pkg + " " + obj.Name() + " " + typ + " " + ult
	case *types.Const:
		line = "CONSTANT: " + pkg + " " + obj.Name() + " " + typ + " " + ult
	case *types.TypeName:
		line = "TYPE:     " + pkg + " " + obj.Name() + " " + typ + " " + ult
	case *types.Builtin:
		return ""
	}
	return line
}
