package buildinfo

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	// Сохраняем оригинальные значения
	origVersion := version
	origDate := date
	origCommit := commit

	// Восстанавливаем после теста
	defer func() {
		version = origVersion
		date = origDate
		commit = origCommit
	}()

	tests := []struct {
		name     string
		version  string
		date     string
		commit   string
		expected BuildInfo
	}{
		{
			name:    "all fields set",
			version: "v1.0.0",
			date:    "2024-01-15 14:30:25",
			commit:  "a1b2c3d",
			expected: BuildInfo{
				Version: "v1.0.0",
				Date:    "2024-01-15 14:30:25",
				Commit:  "a1b2c3d",
			},
		},
		{
			name:    "empty fields",
			version: "",
			date:    "",
			commit:  "",
			expected: BuildInfo{
				Version: "",
				Date:    "",
				Commit:  "",
			},
		},
		{
			name:    "only version set",
			version: "v2.0.0",
			date:    "",
			commit:  "",
			expected: BuildInfo{
				Version: "v2.0.0",
				Date:    "",
				Commit:  "",
			},
		},
		{
			name:    "only date set",
			version: "",
			date:    "2024-02-20 10:15:30",
			commit:  "",
			expected: BuildInfo{
				Version: "",
				Date:    "2024-02-20 10:15:30",
				Commit:  "",
			},
		},
		{
			name:    "only commit set",
			version: "",
			date:    "",
			commit:  "f9e8d7c",
			expected: BuildInfo{
				Version: "",
				Date:    "",
				Commit:  "f9e8d7c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Устанавливаем тестовые значения
			version = tt.version
			date = tt.date
			commit = tt.commit

			// Получаем результат
			result := Get()

			// Проверяем
			if result.Version != tt.expected.Version {
				t.Errorf("Get().Version = %v, want %v", result.Version, tt.expected.Version)
			}
			if result.Date != tt.expected.Date {
				t.Errorf("Get().Date = %v, want %v", result.Date, tt.expected.Date)
			}
			if result.Commit != tt.expected.Commit {
				t.Errorf("Get().Commit = %v, want %v", result.Commit, tt.expected.Commit)
			}
		})
	}
}

func TestPrint(t *testing.T) {
	// Сохраняем оригинальный stdout
	oldStdout := os.Stdout
	// Сохраняем оригинальные значения
	origVersion := version
	origDate := date
	origCommit := commit

	// Восстанавливаем после теста
	defer func() {
		os.Stdout = oldStdout
		version = origVersion
		date = origDate
		commit = origCommit
	}()

	tests := []struct {
		name     string
		version  string
		date     string
		commit   string
		expected string
	}{
		{
			name:     "all fields set",
			version:  "v1.0.0",
			date:     "2024-01-15 14:30:25",
			commit:   "a1b2c3d",
			expected: "Build version: v1.0.0\nBuild date: 2024-01-15 14:30:25\nBuild commit: a1b2c3d\n",
		},
		{
			name:     "all fields empty",
			version:  "",
			date:     "",
			commit:   "",
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:     "only version set",
			version:  "v2.0.0",
			date:     "",
			commit:   "",
			expected: "Build version: v2.0.0\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:     "only date set",
			version:  "",
			date:     "2024-02-20 10:15:30",
			commit:   "",
			expected: "Build version: N/A\nBuild date: 2024-02-20 10:15:30\nBuild commit: N/A\n",
		},
		{
			name:     "only commit set",
			version:  "",
			date:     "",
			commit:   "f9e8d7c",
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: f9e8d7c\n",
		},
		{
			name:     "version with spaces",
			version:  "v1.0.0-beta.1",
			date:     "",
			commit:   "",
			expected: "Build version: v1.0.0-beta.1\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:     "date with different format",
			version:  "",
			date:     "2024-02-20T10:15:30Z",
			commit:   "",
			expected: "Build version: N/A\nBuild date: 2024-02-20T10:15:30Z\nBuild commit: N/A\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Устанавливаем тестовые значения
			version = tt.version
			date = tt.date
			commit = tt.commit

			// Создаем pipe для перехвата stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Вызываем тестируемую функцию
			Print()

			// Закрываем writer и восстанавливаем stdout
			w.Close()
			os.Stdout = oldStdout

			// Читаем вывод
			var buf bytes.Buffer
			io.Copy(&buf, r)
			r.Close()

			// Проверяем результат
			if buf.String() != tt.expected {
				t.Errorf("Print() output = %q, want %q", buf.String(), tt.expected)
			}
		})
	}
}

func TestString(t *testing.T) {
	// Сохраняем оригинальные значения
	origVersion := version
	origDate := date
	origCommit := commit

	// Восстанавливаем после теста
	defer func() {
		version = origVersion
		date = origDate
		commit = origCommit
	}()

	tests := []struct {
		name     string
		version  string
		date     string
		commit   string
		expected string
	}{
		{
			name:     "all fields set",
			version:  "v1.0.0",
			date:     "2024-01-15 14:30:25",
			commit:   "a1b2c3d",
			expected: "Build version: v1.0.0\nBuild date: 2024-01-15 14:30:25\nBuild commit: a1b2c3d",
		},
		{
			name:     "all fields empty",
			version:  "",
			date:     "",
			commit:   "",
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A",
		},
		{
			name:     "only version set",
			version:  "v2.0.0",
			date:     "",
			commit:   "",
			expected: "Build version: v2.0.0\nBuild date: N/A\nBuild commit: N/A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Устанавливаем тестовые значения
			version = tt.version
			date = tt.date
			commit = tt.commit

			// Получаем результат
			result := String()

			// Проверяем
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildInfo_String(t *testing.T) {
	tests := []struct {
		name     string
		info     BuildInfo
		expected string
	}{
		{
			name: "all fields set",
			info: BuildInfo{
				Version: "v1.0.0",
				Date:    "2024-01-15 14:30:25",
				Commit:  "a1b2c3d",
			},
			expected: "Build version: v1.0.0\nBuild date: 2024-01-15 14:30:25\nBuild commit: a1b2c3d",
		},
		{
			name: "all fields empty",
			info: BuildInfo{
				Version: "",
				Date:    "",
				Commit:  "",
			},
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.String()
			if result != tt.expected {
				t.Errorf("BuildInfo.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Тест на безопасность конкурентного доступа
	done := make(chan bool)

	// Запускаем несколько горутин для чтения
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				Get()
				String()
			}
			done <- true
		}()
	}

	// Ждем завершения всех горутин
	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Get()
	}
}

func BenchmarkString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		String()
	}
}

func BenchmarkPrint(b *testing.B) {
	// Сохраняем оригинальный stdout
	oldStdout := os.Stdout

	// Перенаправляем stdout в null для бенчмарка
	os.Stdout, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Print()
	}

	// Восстанавливаем stdout
	os.Stdout = oldStdout
}
