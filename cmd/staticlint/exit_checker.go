package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

var OsExitAnalyzer = &analysis.Analyzer{
	Name: "osexit",
	Doc:  "check whether os.Exit is used in main",
	Run:  runAnalyzer,
}

type osExitVisitor struct {
	Pass *analysis.Pass
}

func (o *osExitVisitor) Visit(node ast.Node) ast.Visitor {
	if o.Pass.Pkg.Name() != "main" {
		return nil
	}

	switch x := node.(type) {
	case *ast.FuncDecl:
		if x.Name.Name != "main" {
			return nil
		}
	case *ast.CallExpr:
		o.callExprVisitor(x)
	}
	return o
}

func (o *osExitVisitor) callExprVisitor(c *ast.CallExpr) {
	if fun, ok := c.Fun.(*ast.SelectorExpr); ok {
		pkg, ok := fun.X.(*ast.Ident)
		if !ok || pkg.Name != "os" || fun.Sel.Name != "Exit" {
			return
		}
		o.Pass.Reportf(fun.Pos(), "use os.Exit")
	}
}

func runAnalyzer(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Walk(&osExitVisitor{Pass: pass}, file)
	}
	return nil, nil
}
