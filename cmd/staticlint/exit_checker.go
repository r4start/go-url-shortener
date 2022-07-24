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

func callExprVisitor(pass *analysis.Pass, c *ast.CallExpr) {
	if fun, ok := c.Fun.(*ast.SelectorExpr); ok {
		pkg, ok := fun.X.(*ast.Ident)
		if !ok || pkg.Name != "os" || fun.Sel.Name != "Exit" {
			return
		}
		pass.Reportf(fun.Pos(), "use os.Exit")
	}
}

func runAnalyzer(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.CallExpr:
				callExprVisitor(pass, x)
			}
			return true
		})
	}
	return nil, nil
}
