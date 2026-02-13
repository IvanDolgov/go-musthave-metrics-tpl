package main

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNoExitAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()

	// Создаем временный модуль для тестов
	t.Setenv("GO111MODULE", "on")

	analysistest.Run(t, testdata, NoExitAnalyzer, "noexit")
}
