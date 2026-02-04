package typegen

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/coder/guts"
	"github.com/coder/guts/config"
)

type Options struct {
	PkgPath        string
	PkgDir         string
	IncludePattern string
	IncludeType    string
}

type OutputOptions struct {
	OutputPath string
	Stdout     bool
}

const defaultOutputFile = "index.d.ts"

func DefaultOutputPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return defaultOutputFile
	}
	return filepath.Join(filepath.Dir(exePath), defaultOutputFile)
}

// GenerateTypes generates TypeScript types from Go structs
func GenerateTypes(pkgPath string) (string, error) {
	return GenerateTypesWithOptions(Options{
		PkgPath: pkgPath,
	})
}

// GenerateTypesWithOptions generates TypeScript types with custom configuration.
func GenerateTypesWithOptions(opts Options) (string, error) {
	if opts.PkgDir == "" {
		return "", fmt.Errorf("pkg-dir is required")
	}

	pkgDir, err := resolvePkgDir(opts.PkgDir)
	if err != nil {
		return "", fmt.Errorf("resolve pkg dir: %w", err)
	}

	pkgImportPath, err := resolvePkgPath(pkgDir, opts.PkgPath)
	if err != nil {
		return "", fmt.Errorf("resolve pkg import path: %w", err)
	}

	packages, err := findPackages(pkgDir, pkgImportPath)
	if err != nil {
		return "", fmt.Errorf("find packages: %w", err)
	}

	interfaceTypes, err := collectInterfaceTypeNames(pkgDir, pkgImportPath)
	if err != nil {
		return "", fmt.Errorf("collect interface types: %w", err)
	}

	// 使用单一 parser 处理所有包，确保跨包引用正确解析
	golang, err := guts.NewGolangParser()
	if err != nil {
		return "", fmt.Errorf("create parser: %w", err)
	}

	golang.PreserveComments()
	golang.IncludeCustomDeclaration(config.StandardMappings())

	for _, pkg := range packages {
		prefix := prefixForImportPath(pkgImportPath, pkg.importPath)
		if err := golang.IncludeGenerateWithPrefix(pkg.importPath, prefix); err != nil {
			// Skip packages that fail (may have no Go files)
			continue
		}
	}

	ts, err := golang.ToTypescript()
	if err != nil {
		return "", fmt.Errorf("convert to typescript: %w", err)
	}

	ts.ApplyMutations(
		config.ExportTypes,
		config.EnumAsTypes,
		config.ReadOnly,
		config.NullUnionSlices,
		config.NotNullMaps,
		config.BiomeLintIgnoreAnyTypeParameters,
	)

	output, err := ts.Serialize()
	if err != nil {
		return "", fmt.Errorf("serialize: %w", err)
	}

	if opts.IncludePattern != "" || opts.IncludeType != "" {
		var fileRegexp *regexp.Regexp
		var typeRegexp *regexp.Regexp
		var err error
		if opts.IncludePattern != "" {
			fileRegexp, err = regexp.Compile(opts.IncludePattern)
			if err != nil {
				return "", fmt.Errorf("compile include pattern: %w", err)
			}
		}
		if opts.IncludeType != "" {
			typeRegexp, err = regexp.Compile(opts.IncludeType)
			if err != nil {
				return "", fmt.Errorf("compile include type pattern: %w", err)
			}
		}
		output = filterByWhitelist(output, fileRegexp, typeRegexp)
	}

	output = filterInterfaceTypes(output, interfaceTypes)
	output = deduplicateTypes(output)

	return output, nil
}

func GenerateTypesToOutput(opts Options, output OutputOptions) error {
	content, err := GenerateTypesWithOptions(opts)
	if err != nil {
		return err
	}

	if output.OutputPath == "" {
		output.OutputPath = DefaultOutputPath()
	}

	if output.Stdout || output.OutputPath == "-" {
		_, err := os.Stdout.WriteString(content)
		return err
	}

	outPath := filepath.Clean(output.OutputPath)
	if outPath == "" {
		return fmt.Errorf("output path is required unless stdout is set")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("ensure output directory: %w", err)
	}

	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", outPath, err)
	}

	return nil
}

type packageInfo struct {
	importPath string
}

// findPackages discovers all packages under pkgDir
func findPackages(pkgDir, pkgImportPath string) ([]packageInfo, error) {
	var packages []packageInfo
	seen := make(map[string]struct{})

	err := filepath.WalkDir(pkgDir, func(dir string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}

		name := entry.Name()
		if dir != pkgDir && (name == "typegen" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}

		hasGoFiles := false
		for _, file := range entries {
			if file.IsDir() {
				continue
			}
			fileName := file.Name()
			if strings.HasSuffix(fileName, ".go") && !strings.HasSuffix(fileName, "_test.go") {
				hasGoFiles = true
				break
			}
		}

		if !hasGoFiles {
			return nil
		}

		rel, err := filepath.Rel(pkgDir, dir)
		if err != nil {
			return err
		}

		importPath := pkgImportPath
		if rel != "." {
			importPath = path.Join(pkgImportPath, filepath.ToSlash(rel))
		}

		if _, ok := seen[importPath]; !ok {
			seen[importPath] = struct{}{}
			packages = append(packages, packageInfo{importPath: importPath})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk pkg dir: %w", err)
	}

	// Sort for deterministic output
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].importPath < packages[j].importPath
	})

	return packages, nil
}

func collectInterfaceTypeNames(pkgDir, pkgImportPath string) (map[string]struct{}, error) {
	interfaces := make(map[string]struct{})
	fset := token.NewFileSet()

	err := filepath.WalkDir(pkgDir, func(dir string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}

		name := entry.Name()
		if dir != pkgDir && (name == "typegen" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}

		var goFiles []string
		for _, file := range entries {
			if file.IsDir() {
				continue
			}
			fileName := file.Name()
			if strings.HasSuffix(fileName, ".go") && !strings.HasSuffix(fileName, "_test.go") {
				goFiles = append(goFiles, filepath.Join(dir, fileName))
			}
		}

		if len(goFiles) == 0 {
			return nil
		}

		rel, err := filepath.Rel(pkgDir, dir)
		if err != nil {
			return err
		}

		importPath := pkgImportPath
		if rel != "." {
			importPath = path.Join(pkgImportPath, filepath.ToSlash(rel))
		}
		prefix := prefixForImportPath(pkgImportPath, importPath)

		for _, filePath := range goFiles {
			parsed, err := parser.ParseFile(fset, filePath, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse file %s: %w", filePath, err)
			}

			for _, decl := range parsed.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, ok := typeSpec.Type.(*ast.InterfaceType); !ok {
						continue
					}
					name := typeSpec.Name.Name
					if name == "" || !ast.IsExported(name) {
						continue
					}
					interfaces[prefix+name] = struct{}{}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk pkg dir: %w", err)
	}

	return interfaces, nil
}

func filterByWhitelist(content string, fileRegexp, typeRegexp *regexp.Regexp) string {
	if fileRegexp == nil && typeRegexp == nil {
		return content
	}

	type tsBlock struct {
		source string
		lines  []string
		name   string
	}

	lines := strings.Split(content, "\n")
	var header []string
	var blocks []tsBlock
	var current *tsBlock

	for _, line := range lines {
		if strings.HasPrefix(line, "// From ") {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &tsBlock{
				source: strings.TrimPrefix(line, "// From "),
				lines:  []string{line},
			}
			continue
		}

		if current == nil {
			header = append(header, line)
			continue
		}

		current.lines = append(current.lines, line)
	}

	if current != nil {
		blocks = append(blocks, *current)
	}

	exportNames := make(map[string]struct{})
	for i := range blocks {
		for _, line := range blocks[i].lines {
			name := extractExportName(line)
			if name != "" {
				blocks[i].name = name
				exportNames[name] = struct{}{}
				break
			}
		}
	}

	tokenRe := regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)
	refsByName := make(map[string]map[string]struct{})
	for _, block := range blocks {
		if block.name == "" {
			continue
		}
		refs := make(map[string]struct{})
		for _, line := range block.lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			for _, token := range tokenRe.FindAllString(line, -1) {
				if token == block.name {
					continue
				}
				if _, ok := exportNames[token]; ok {
					refs[token] = struct{}{}
				}
			}
		}
		refsByName[block.name] = refs
	}

	selected := make(map[string]struct{})
	queue := make([]string, 0)
	for _, block := range blocks {
		if block.name == "" {
			continue
		}
		if matchesWhitelist(block.source, block.name, fileRegexp, typeRegexp) {
			if _, ok := selected[block.name]; !ok {
				selected[block.name] = struct{}{}
				queue = append(queue, block.name)
			}
		}
	}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for ref := range refsByName[name] {
			if _, ok := selected[ref]; ok {
				continue
			}
			selected[ref] = struct{}{}
			queue = append(queue, ref)
		}
	}

	result := append([]string{}, header...)
	for _, block := range blocks {
		if block.name == "" {
			continue
		}
		if _, ok := selected[block.name]; ok {
			result = append(result, block.lines...)
		}
	}

	return strings.Join(result, "\n")
}

func matchesWhitelist(source, name string, fileRegexp, typeRegexp *regexp.Regexp) bool {
	if fileRegexp != nil && !fileRegexp.MatchString(source) {
		return false
	}
	if typeRegexp == nil {
		return true
	}
	if name == "" {
		return false
	}
	return typeRegexp.MatchString(name)
}

func filterInterfaceTypes(content string, excluded map[string]struct{}) string {
	if len(excluded) == 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	var pending []string
	var current []string
	var currentName string
	seenType := false

	flushCurrent := func() {
		if current == nil {
			return
		}
		if _, skip := excluded[currentName]; !skip {
			result = append(result, current...)
		}
		current = nil
		currentName = ""
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "export interface ") || strings.HasPrefix(line, "export type ") {
			if current != nil {
				flushCurrent()
			} else if !seenType && len(pending) > 0 {
				result = append(result, pending...)
				pending = nil
			}

			seenType = true
			currentName = extractTypeName(line)
			current = append(current, pending...)
			current = append(current, line)
			pending = nil
			continue
		}

		if current != nil {
			current = append(current, line)
			continue
		}

		if seenType {
			pending = append(pending, line)
		} else {
			result = append(result, line)
		}
	}

	if current != nil {
		flushCurrent()
	} else if !seenType && len(pending) > 0 {
		result = append(result, pending...)
	}

	return strings.Join(result, "\n")
}

// deduplicateTypes removes duplicate type definitions (keep first occurrence)
func deduplicateTypes(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	seen := make(map[string]bool)
	skip := false
	var pendingComment []string

	for _, line := range lines {
		// Detect type definition start
		if strings.HasPrefix(line, "export interface ") || strings.HasPrefix(line, "export type ") {
			typeName := extractTypeName(line)
			if seen[typeName] {
				skip = true
				pendingComment = nil
				continue
			}
			seen[typeName] = true
			skip = false
			result = append(result, pendingComment...)
			pendingComment = nil
			result = append(result, line)
			continue
		}

		// Collect comments (may belong to next type)
		if strings.HasPrefix(line, "// From ") || strings.HasPrefix(line, "/**") ||
			strings.HasPrefix(line, " *") || strings.HasPrefix(line, " */") {
			if skip {
				continue
			}
			pendingComment = append(pendingComment, line)
			continue
		}

		// Empty line
		if line == "" {
			if skip {
				skip = false
				continue
			}
			if len(pendingComment) > 0 {
				pendingComment = append(pendingComment, line)
			} else {
				result = append(result, line)
			}
			continue
		}

		// Type content
		if skip {
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func extractTypeName(line string) string {
	line = strings.TrimPrefix(line, "export interface ")
	line = strings.TrimPrefix(line, "export type ")
	for i, ch := range line {
		if ch == ' ' || ch == '{' || ch == '=' || ch == '<' {
			return line[:i]
		}
	}
	return line
}

func extractExportName(line string) string {
	prefixes := []string{
		"export interface ",
		"export type ",
		"export const ",
		"export enum ",
		"export class ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			rest := strings.TrimPrefix(line, prefix)
			for i, ch := range rest {
				if ch == ' ' || ch == '{' || ch == '=' || ch == '<' || ch == '(' {
					return rest[:i]
				}
			}
			return rest
		}
	}
	return ""
}

func prefixForImportPath(pkgImportPath, importPath string) string {
	rel := strings.TrimPrefix(importPath, pkgImportPath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return ""
	}

	prefix := strings.ReplaceAll(rel, "/", "__")
	prefix = strings.ReplaceAll(prefix, "-", "_")
	prefix = strings.ReplaceAll(prefix, ".", "_")
	prefix = strings.ReplaceAll(prefix, "@", "_")
	if prefix == "" {
		return ""
	}

	first := prefix[0]
	if !(first == '_' || (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')) {
		prefix = "pkg_" + prefix
	}

	return prefix + "_"
}

func resolvePkgDir(explicit string) (string, error) {
	if explicit == "" {
		return "", fmt.Errorf("pkg-dir is required")
	}

	abs, err := filepath.Abs(explicit)
	if err != nil {
		return "", fmt.Errorf("abs pkg directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat pkg directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("pkg path %s is not a directory", abs)
	}
	return abs, nil
}

func resolvePkgPath(pkgDir, pkgPath string) (string, error) {
	if pkgPath != "" {
		return strings.TrimSuffix(pkgPath, "/"), nil
	}

	modulePath, err := findModulePath(pkgDir)
	if err != nil {
		return "", err
	}

	return path.Join(modulePath, "pkg"), nil
}

func findModulePath(startDir string) (string, error) {
	dir := startDir
	for {
		modPath := filepath.Join(dir, "go.mod")
		info, err := os.Stat(modPath)
		if err == nil && !info.IsDir() {
			data, err := os.ReadFile(modPath)
			if err != nil {
				return "", fmt.Errorf("read go.mod: %w", err)
			}
			modulePath, err := parseModulePath(data)
			if err != nil {
				return "", fmt.Errorf("parse module path from %s: %w", modPath, err)
			}
			return modulePath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("go.mod not found from %s", startDir)
}

func parseModulePath(data []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("module directive not found")
}
