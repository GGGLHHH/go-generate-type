# go-generate-type

Generate TypeScript types from Go structs using [coder/guts](https://github.com/coder/guts).

## Install

```bash
go install github.com/GGGLHHH/go-generate-type/cmd/typegen@latest
```

## Usage

```bash
# required: path to the Go pkg root you want to scan
# optional: output file path, whitelist by file name / type name

typegen \
  -pkg-dir ./pkg \
  -out ./index.d.ts \
  -include 'dto\\.go$' \
  -include-type 'Req$|Res$'
```

### Flags

- `-pkg-dir` (required): filesystem path to the root `pkg` directory you want to scan.
- `-pkg-path` (optional): Go import path that corresponds to `-pkg-dir` (defaults to `<module>/pkg`).
- `-include` / `-include-file` (optional): regex for source file paths to include.
- `-include-type` (optional): regex for exported type names to include.
- `-strip-prefix` (optional): remove package prefixes from generated identifiers.
- `-disable-rename` (optional): skip rename scan (TypeNameMapper ignored).
- `-out` / `-out-file` (optional): output file path (defaults to `index.d.ts` next to the executable).
- `-stdout` (optional): write to stdout instead of a file.

### Whitelist behavior

When both `-include` and `-include-type` are set, the output uses their **intersection**.
Types referenced by matched types are included automatically (dependency closure) to avoid missing definitions.

## Library usage

```go
package main

import (
    "log"

    "github.com/GGGLHHH/go-generate-type/pkg/typegen"
)

func main() {
    if err := typegen.GenerateTypesToOutput(typegen.Options{
        PkgDir: "./pkg",
        // StripPrefix removes package prefixes like "foo__bar_".
        StripPrefix: true,
    }, typegen.OutputOptions{}); err != nil {
        log.Fatal(err)
    }
}
```

### Options (library)

`typegen.Options` lets you customize behavior beyond the CLI defaults:

- `PkgDir` (required): path to the `pkg` directory.
- `PkgPath` (optional): import path for `PkgDir` (`<module>/pkg` if empty).
- `IncludePattern`: regex matched against the "From <pkg>/<file>" header.
- `IncludeType`: regex matched against exported type names (after rename/prefix stripping).
- `StripPrefix`: remove package prefixes from identifiers.
- `DisableRename`: skip rename scan to avoid collisions (TypeNameMapper ignored).
- `TypeNameMapper`: optional mapper for custom TypeScript names.

When both include patterns are provided, the generator keeps their intersection and
automatically includes referenced types.

### Presets (library)

Use `typegen.Preset` to store project defaults and build `Options` consistently:

```go
preset := typegen.Preset{
    IncludePattern: `^dto/`,
    StripPrefix:    true,
    DisableRename:  true,
}

opts := preset.Options("./pkg", "example.com/project/pkg")
```
