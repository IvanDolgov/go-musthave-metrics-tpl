/*
Package main предоставляет multichecker для статического анализа Go кода.

# Использование

	# Проверить текущий пакет
	go run cmd/staticlint/main.go .

	# Проверить все поддиректории
	go run cmd/staticlint/main.go ./...

	# Собрать бинарник
	go build -o staticlint cmd/staticlint/main.go
	./staticlint ./...

# Включаемые анализаторы

## 1. Стандартные анализаторы из golang.org/x/tools/go/analysis/passes
## 2. Все SA-анализаторы из staticcheck (Security/Performance)
## 3. Дополнительные анализаторы из staticcheck (ST1000, S1000)
## 4. Публичные анализаторы: errcheck, unused, ineffassign
## 5. Кастомный анализатор: запрещает прямой вызов os.Exit в функции main
*/
package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/fieldalignment"
	"golang.org/x/tools/go/analysis/passes/findcall"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"

	// Простые и совместимые анализаторы
	"github.com/gordonklaus/ineffassign/pkg/ineffassign" // Неэффективные присваивания
	"github.com/kisielk/errcheck/errcheck"               // Проверка необработанных ошибок
	"honnef.co/go/tools/staticcheck"
)

func main() {
	// 1. Собираем все анализаторы
	checks := []*analysis.Analyzer{
		// Кастомный анализатор (определен в analyzer.go)
		NoExitAnalyzer,

		// Стандартные анализаторы
		asmdecl.Analyzer,
		assign.Analyzer,
		atomic.Analyzer,
		bools.Analyzer,
		buildtag.Analyzer,
		cgocall.Analyzer,
		composite.Analyzer,
		copylock.Analyzer,
		deepequalerrors.Analyzer,
		errorsas.Analyzer,
		fieldalignment.Analyzer,
		findcall.Analyzer,
		httpresponse.Analyzer,
		ifaceassert.Analyzer,
		loopclosure.Analyzer,
		lostcancel.Analyzer,
		nilfunc.Analyzer,
		nilness.Analyzer,
		printf.Analyzer,
		shadow.Analyzer,
		sortslice.Analyzer,
		stdmethods.Analyzer,
		stringintconv.Analyzer,
		structtag.Analyzer,
		testinggoroutine.Analyzer,
		tests.Analyzer,
		unmarshal.Analyzer,
		unreachable.Analyzer,
		unsafeptr.Analyzer,
		unusedresult.Analyzer,

		// Публичные анализаторы
		errcheck.Analyzer,
		ineffassign.Analyzer,
	}

	// 2. Добавляем все SA анализаторы из staticcheck
	for _, v := range staticcheck.Analyzers {
		if len(v.Analyzer.Name) >= 2 && v.Analyzer.Name[:2] == "SA" {
			checks = append(checks, v.Analyzer)
		}
	}

	// 3. Добавляем минимум 2 анализатора из других классов staticcheck
	nonSACount := 0
	for _, v := range staticcheck.Analyzers {
		if len(v.Analyzer.Name) >= 2 && v.Analyzer.Name[:2] != "SA" {
			if v.Analyzer.Name == "ST1000" || v.Analyzer.Name == "S1000" {
				checks = append(checks, v.Analyzer)
				nonSACount++
				if nonSACount >= 2 {
					break
				}
			}
		}
	}

	// Если не нашли ST1000 или S1000, берем любые другие
	if nonSACount < 2 {
		for _, v := range staticcheck.Analyzers {
			if len(v.Analyzer.Name) >= 2 && v.Analyzer.Name[:2] != "SA" && nonSACount < 2 {
				// Проверяем, что еще не добавили этот анализатор
				alreadyAdded := false
				for _, check := range checks {
					if check.Name == v.Analyzer.Name {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					checks = append(checks, v.Analyzer)
					nonSACount++
				}
			}
		}
	}

	// 4. Запускаем multichecker
	multichecker.Main(checks...)
}
