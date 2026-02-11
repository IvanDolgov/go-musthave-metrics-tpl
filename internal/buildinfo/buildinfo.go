// Package buildinfo предоставляет информацию о сборке приложения
package buildinfo

import "fmt"

// BuildInfo содержит информацию о версии сборки
type BuildInfo struct {
	Version string
	Date    string
	Commit  string
}

// String возвращает информацию о сборке в виде строки
func (b BuildInfo) String() string {
	version := b.Version
	date := b.Date
	commit := b.Commit

	if version == "" {
		version = "N/A"
	}
	if date == "" {
		date = "N/A"
	}
	if commit == "" {
		commit = "N/A"
	}

	return fmt.Sprintf("Build version: %s\nBuild date: %s\nBuild commit: %s",
		version, date, commit)
}

var (
	// version устанавливается при сборке через -ldflags
	version string
	// date устанавливается при сборке через -ldflags
	date string
	// commit устанавливается при сборке через -ldflags
	commit string
)

// Get возвращает информацию о сборке
func Get() BuildInfo {
	return BuildInfo{
		Version: version,
		Date:    date,
		Commit:  commit,
	}
}

// Print выводит информацию о сборке в stdout в требуемом формате
func Print() {
	fmt.Print(Get().String() + "\n")
}

// String возвращает информацию о сборке в виде строки
func String() string {
	return Get().String()
}
