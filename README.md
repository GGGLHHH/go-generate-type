# go-generate-type

Generate TypeScript types from Go structs using [coder/guts](https://github.com/coder/guts).

## Install

```bash
go install github.com/17359898647/go-generate-type/cmd/typegen@latest
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

    "github.com/17359898647/go-generate-type/pkg/typegen"
)

func main() {
    if err := typegen.GenerateTypesToOutput(typegen.Options{
        PkgDir: "./pkg",
    }, typegen.OutputOptions{}); err != nil {
        log.Fatal(err)
    }
}
```
