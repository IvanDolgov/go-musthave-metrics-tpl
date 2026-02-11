// Package buildinfo предоставляет информацию о сборке приложения
package buildinfo

import "fmt"

// BuildInfo содержит информацию о версии сборки
type BuildInfo struct {
	Version string
	Date    string
	Commit  string
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
	info := Get()

	// Определяем значения или "N/A" если они пустые
	version := info.Version
	date := info.Date
	commit := info.Commit

	if version == "" {
		version = "N/A"
	}
	if date == "" {
		date = "N/A"
	}
	if commit == "" {
		commit = "N/A"
	}

	// Выводим в stdout
	fmt.Printf("Build version: %s\n", version)
	fmt.Printf("Build date: %s\n", date)
	fmt.Printf("Build commit: %s\n", commit)
}

// String возвращает информацию о сборке в виде строки
func String() string {
	info := Get()

	version := info.Version
	date := info.Date
	commit := info.Commit

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
