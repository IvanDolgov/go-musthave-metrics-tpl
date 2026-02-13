package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// NoExitAnalyzer запрещает прямой вызов os.Exit в функции main пакета main.
var NoExitAnalyzer = &analysis.Analyzer{
	Name: "noexit",
	Doc:  "запрещает прямой вызов os.Exit в функции main пакета main",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Проверяем, что мы в пакете main
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			// Ищем вызовы функций
			callExpr, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Проверяем, что это вызов функции с селектором (например, os.Exit)
			fun, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Проверяем, что это вызов os.Exit
			ident, ok := fun.X.(*ast.Ident)
			if !ok || ident.Name != "os" || fun.Sel.Name != "Exit" {
				return true
			}

			// Ищем, находится ли этот вызов в функции main
			for _, decl := range file.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if !ok || funcDecl.Name.Name != "main" {
					continue
				}

				// Проверяем, находится ли вызов внутри этой функции
				if funcDecl.Pos() <= callExpr.Pos() && callExpr.Pos() <= funcDecl.End() {
					pass.Reportf(callExpr.Pos(),
						"прямой вызов os.Exit в функции main запрещен. Используйте return из функции main")
				}
			}

			return true
		})
	}

	return nil, nil
}
