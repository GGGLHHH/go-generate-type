package typegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTypes_WhitelistDependencyClosure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/test\n\ngo 1.25.0\n")

	writeFile(t, root, "pkg/foo/dto.go", `package foo

type FooReq struct {
	Bar Bar
}

// FooRes should not be included by type whitelist
// unless referenced.
type FooRes struct {
	ID int
}
`)
	writeFile(t, root, "pkg/foo/bar.go", `package foo

type Bar struct {
	Name string
}
`)
	writeFile(t, root, "pkg/foo/iface.go", `package foo

type Transaction interface {
	Commit() error
}

type Embedded interface {
	Transaction
}
`)
	writeFile(t, root, "pkg/baz/other.go", `package baz

type Other struct {
	Value string
}
`)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	t.Setenv("GOMOD", filepath.Join(root, "go.mod"))
	t.Setenv("GOWORK", "off")

	raw, err := GenerateTypesWithOptions(Options{
		PkgDir: filepath.Join(root, "pkg"),
	})
	if err != nil {
		t.Fatalf("GenerateTypesWithOptions (raw): %v", err)
	}
	if !strings.Contains(raw, "FooReq") {
		t.Fatalf("expected FooReq in raw output")
	}

	output, err := GenerateTypesWithOptions(Options{
		PkgDir:         filepath.Join(root, "pkg"),
		IncludePattern: `^foo/`,
		IncludeType:    `Req$`,
	})
	if err != nil {
		t.Fatalf("GenerateTypesWithOptions: %v", err)
	}

	if !strings.Contains(output, "FooReq") {
		t.Fatalf("expected FooReq to be generated")
	}
	if !strings.Contains(output, "Bar") {
		t.Fatalf("expected referenced Bar to be generated")
	}
	if strings.Contains(output, "Other") {
		t.Fatalf("did not expect Other to be generated")
	}
	if strings.Contains(output, "FooRes") {
		t.Fatalf("did not expect FooRes to be generated")
	}
	if strings.Contains(output, "Transaction") || strings.Contains(output, "Embedded") {
		t.Fatalf("did not expect interface types to be generated")
	}
}

func TestGenerateTypes_RequiresPkgDir(t *testing.T) {
	_, err := GenerateTypesWithOptions(Options{})
	if err == nil {
		t.Fatalf("expected error when pkg-dir is empty")
	}
}

func TestGenerateTypes_TypeNameMapper(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/test\n\ngo 1.25.0\n")

	writeFile(t, root, "pkg/foo/dto.go", `package foo

type FooReq struct {
	Bar Bar
}
`)
	writeFile(t, root, "pkg/foo/bar.go", `package foo

type Bar struct {
	Name string
}
`)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	t.Setenv("GOMOD", filepath.Join(root, "go.mod"))
	t.Setenv("GOWORK", "off")

	output, err := GenerateTypesWithOptions(Options{
		PkgDir: filepath.Join(root, "pkg"),
		TypeNameMapper: func(typeName, moduleName string) string {
			return "X" + typeName
		},
	})
	if err != nil {
		t.Fatalf("GenerateTypesWithOptions: %v", err)
	}

	if !strings.Contains(output, "XFooReq") {
		t.Fatalf("expected renamed FooReq to be generated")
	}
	if !strings.Contains(output, "XBar") {
		t.Fatalf("expected renamed Bar to be generated")
	}
	if strings.Contains(output, "foo_FooReq") || strings.Contains(output, "foo_Bar") {
		t.Fatalf("did not expect original prefixed names to remain")
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
