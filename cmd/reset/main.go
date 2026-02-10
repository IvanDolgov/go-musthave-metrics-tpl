/*
Package main предоставляет утилиту для генерации методов Reset().

Утилита сканирует все пакеты, начиная с корневой директории,
находит структуры с комментарием // generate:reset и генерирует
для них методы Reset() в файле reset.gen.go.

# Использование

	# Сгенерировать методы Reset() для всех пакетов
	go run cmd/reset/main.go .

	# Сгенерировать для конкретного пакета
	go run cmd/reset/main.go ./internal/...

# Правила генерации

1. Примитивы сбрасываются к нулевым значениям
2. Слайсы обрезаются до длины 0
3. Мапы очищаются
4. Вложенные структуры вызывают свой Reset() если он есть
5. Указатели сбрасывают значения если не nil
*/
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Структура для хранения информации о поле
type fieldInfo struct {
	name        string // имя поля
	typeExpr    string // тип поля как выражение
	isPtr       bool   // является ли указателем
	isSlice     bool   // является ли слайсом
	isMap       bool   // является ли мапой
	isStruct    bool   // является ли структурой
	isPrimitive bool   // является ли примитивом
}

// Структура для хранения информации о структуре
type structInfo struct {
	name   string      // имя структуры
	fields []fieldInfo // поля структуры
	file   string      // исходный файл
	pkg    string      // пакет
}

// Проверяет, есть ли у структуры комментарий // generate:reset
func hasGenerateResetComment(node ast.Node) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *ast.GenDecl:
		if n.Doc != nil {
			for _, comment := range n.Doc.List {
				if strings.Contains(comment.Text, "generate:reset") {
					return true
				}
			}
		}
	case *ast.TypeSpec:
		if n.Doc != nil {
			for _, comment := range n.Doc.List {
				if strings.Contains(comment.Text, "generate:reset") {
					return true
				}
			}
		}
		// Проверяем комментарий у родительского GenDecl
		return hasGenerateResetComment(node.(*ast.TypeSpec).Doc)
	}
	return false
}

// Извлекает информацию о типе поля
func parseFieldType(expr ast.Expr) fieldInfo {
	info := fieldInfo{}

	switch t := expr.(type) {
	case *ast.Ident:
		info.typeExpr = t.Name
		info.isPrimitive = isPrimitiveType(t.Name)
		info.isStruct = !info.isPrimitive && !isBuiltinType(t.Name)

	case *ast.StarExpr:
		info.isPtr = true
		nested := parseFieldType(t.X)
		info.typeExpr = "*" + nested.typeExpr
		info.isSlice = nested.isSlice
		info.isMap = nested.isMap
		info.isStruct = nested.isStruct
		info.isPrimitive = nested.isPrimitive

	case *ast.ArrayType:
		info.isSlice = true
		if t.Len == nil {
			elemType := parseFieldType(t.Elt)
			info.typeExpr = "[]" + elemType.typeExpr
			info.isStruct = elemType.isStruct
			info.isPrimitive = elemType.isPrimitive
		}

	case *ast.MapType:
		info.isMap = true
		keyType := parseFieldType(t.Key)
		valueType := parseFieldType(t.Value)
		info.typeExpr = fmt.Sprintf("map[%s]%s", keyType.typeExpr, valueType.typeExpr)
		info.isStruct = valueType.isStruct
		info.isPrimitive = valueType.isPrimitive

	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			info.typeExpr = x.Name + "." + t.Sel.Name
			info.isStruct = true
		}
	}

	return info
}

// Проверяет, является ли тип примитивным
func isPrimitiveType(typeName string) bool {
	primitives := map[string]bool{
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true,
		"complex64": true, "complex128": true,
		"byte": true, "rune": true,
		"string": true, "bool": true,
	}
	return primitives[typeName]
}

// Проверяет, является ли тип встроенным
func isBuiltinType(typeName string) bool {
	builtins := map[string]bool{
		"error": true,
		"any":   true,
	}
	return isPrimitiveType(typeName) || builtins[typeName]
}

// Находит все структуры с комментарием // generate:reset в пакете
func findResetableStructs(dir string) ([]structInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга директории %s: %w", dir, err)
	}

	var structs []structInfo

	for pkgName, pkg := range pkgs {
		for filename, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				typeSpec, ok := node.(*ast.TypeSpec)
				if !ok {
					return true
				}

				if !hasGenerateResetComment(typeSpec) {
					return true
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					return true
				}

				info := structInfo{
					name: typeSpec.Name.Name,
					file: filename,
					pkg:  pkgName,
				}

				// Собираем информацию о полях
				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						// Анонимное поле (встраивание)
						continue
					}

					for _, fieldName := range field.Names {
						if !unicode.IsUpper(rune(fieldName.Name[0])) {
							// Пропускаем приватные поля
							continue
						}

						fieldInfo := parseFieldType(field.Type)
						fieldInfo.name = fieldName.Name
						info.fields = append(info.fields, fieldInfo)
					}
				}

				structs = append(structs, info)
				return true
			})
		}
	}

	return structs, nil
}

// Генерирует код метода Reset() для структуры
func generateResetMethod(info structInfo) string {
	var buf bytes.Buffer

	// Шапка метода
	buf.WriteString(fmt.Sprintf("// Code generated by cmd/reset. DO NOT EDIT.\n\n"))
	buf.WriteString(fmt.Sprintf("package %s\n\n", info.pkg))
	buf.WriteString(fmt.Sprintf("func (rs *%s) Reset() {\n", info.name))
	buf.WriteString("    if rs == nil {\n")
	buf.WriteString("        return\n")
	buf.WriteString("    }\n\n")

	// Генерация сброса для каждого поля
	for _, field := range info.fields {
		buf.WriteString(fmt.Sprintf("    // Поле %s\n", field.name))

		switch {
		case field.isPrimitive:
			if field.isPtr {
				buf.WriteString(fmt.Sprintf("    if rs.%s != nil {\n", field.name))
				buf.WriteString(fmt.Sprintf("        *rs.%s = %s\n", field.name, zeroValue(field.typeExpr[1:])))
				buf.WriteString("    }\n")
			} else {
				buf.WriteString(fmt.Sprintf("    rs.%s = %s\n", field.name, zeroValue(field.typeExpr)))
			}

		case field.isSlice:
			buf.WriteString(fmt.Sprintf("    rs.%s = rs.%s[:0]\n", field.name, field.name))

		case field.isMap:
			buf.WriteString(fmt.Sprintf("    clear(rs.%s)\n", field.name))

		case field.isStruct:
			if field.isPtr {
				buf.WriteString(fmt.Sprintf("    if rs.%s != nil {\n", field.name))
				buf.WriteString(fmt.Sprintf("        if resetter, ok := interface{}(rs.%s).(interface{ Reset() }); ok {\n", field.name))
				buf.WriteString(fmt.Sprintf("            resetter.Reset()\n"))
				buf.WriteString(fmt.Sprintf("        }\n"))
				buf.WriteString(fmt.Sprintf("    }\n"))
			} else {
				buf.WriteString(fmt.Sprintf("    if resetter, ok := interface{}(&rs.%s).(interface{ Reset() }); ok {\n", field.name))
				buf.WriteString(fmt.Sprintf("        resetter.Reset()\n"))
				buf.WriteString(fmt.Sprintf("    }\n"))
			}

		default:
			// Для других типов (интерфейсы, каналы и т.д.)
			if field.isPtr {
				buf.WriteString(fmt.Sprintf("    rs.%s = nil\n", field.name))
			} else {
				buf.WriteString(fmt.Sprintf("    // Поле %s типа %s требует ручной обработки\n",
					field.name, field.typeExpr))
			}
		}
		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	return buf.String()
}

// Возвращает нулевое значение для типа
func zeroValue(typeName string) string {
	switch typeName {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"complex64", "complex128",
		"byte", "rune":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	default:
		// Для структур и других типов
		if strings.HasPrefix(typeName, "*") {
			return "nil"
		}
		if strings.HasPrefix(typeName, "[]") {
			return "nil"
		}
		if strings.HasPrefix(typeName, "map[") {
			return "nil"
		}
		return typeName + "{}"
	}
}

// Форматирует и записывает сгенерированный код
func writeGeneratedFile(dir, content string) error {
	// Форматируем код
	formatted, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("ошибка форматирования кода: %w", err)
	}

	// Создаем директорию если нужно
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("ошибка создания директории: %w", err)
	}

	// Записываем файл
	filename := filepath.Join(dir, "reset.gen.go")
	if err := os.WriteFile(filename, formatted, 0644); err != nil {
		return fmt.Errorf("ошибка записи файла: %w", err)
	}

	fmt.Printf("Создан файл: %s\n", filename)
	return nil
}

// Рекурсивно сканирует директории
func scanPackages(rootDir string) (map[string][]structInfo, error) {
	result := make(map[string][]structInfo)

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Пропускаем скрытые директории и vendor
		if d.IsDir() {
			if strings.Contains(path, ".git") ||
				strings.Contains(path, "vendor") ||
				strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}

			// Проверяем, есть ли go файлы в директории
			hasGoFiles := false
			entries, _ := os.ReadDir(path)
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
					hasGoFiles = true
					break
				}
			}

			if hasGoFiles {
				structs, err := findResetableStructs(path)
				if err != nil {
					return err
				}

				if len(structs) > 0 {
					relPath, _ := filepath.Rel(rootDir, path)
					if relPath == "." {
						relPath = ""
					}
					result[relPath] = structs
				}
			}
		}

		return nil
	})

	return result, err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: go run cmd/reset/main.go <директория>")
		fmt.Println("Пример: go run cmd/reset/main.go .")
		os.Exit(1)
	}

	rootDir := os.Args[1]
	if rootDir == "." {
		rootDir, _ = os.Getwd()
	}

	fmt.Printf("Сканирование директории: %s\n", rootDir)

	// Сканируем все пакеты
	packages, err := scanPackages(rootDir)
	if err != nil {
		fmt.Printf("Ошибка сканирования: %v\n", err)
		os.Exit(1)
	}

	if len(packages) == 0 {
		fmt.Println("Структуры с комментарием // generate:reset не найдены")
		return
	}

	// Генерируем методы для каждого пакета
	for relPath, structs := range packages {
		dir := rootDir
		if relPath != "" {
			dir = filepath.Join(rootDir, relPath)
		}

		// Генерируем файл для пакета
		var content bytes.Buffer
		content.WriteString("// Code generated by cmd/reset. DO NOT EDIT.\n\n")
		content.WriteString(fmt.Sprintf("package %s\n\n", filepath.Base(dir)))

		// Добавляем импорты если нужны
		hasTime := false
		for _, info := range structs {
			for _, field := range info.fields {
				if strings.Contains(field.typeExpr, "time.") {
					hasTime = true
				}
			}
		}

		if hasTime {
			content.WriteString("import \"time\"\n\n")
		}

		// Генерируем методы для каждой структуры
		for _, info := range structs {
			methodContent := generateResetMethod(info)
			// Удаляем package строку из сгенерированного метода
			methodContent = strings.SplitN(methodContent, "\n\n", 2)[1]
			content.WriteString(methodContent)
			content.WriteString("\n")
		}

		// Записываем файл
		if err := writeGeneratedFile(dir, content.String()); err != nil {
			fmt.Printf("Ошибка генерации для пакета %s: %v\n", relPath, err)
		} else {
			fmt.Printf("Сгенерированы методы для пакета: %s\n", relPath)
			for _, info := range structs {
				fmt.Printf("  - %s\n", info.name)
			}
		}
	}

	fmt.Println("Генерация завершена!")
}
