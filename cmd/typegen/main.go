package main

import (
	"flag"
	"log"

	"github.com/GGGLHHH/go-generate-type/pkg/typegen"
)

func main() {
	var opts typegen.Options
	var outputPath string
	var toStdout bool

	flag.StringVar(&opts.PkgPath, "pkg-path", "", "Go module import path for pkg root (default: <module>/pkg)")
	flag.StringVar(&opts.PkgDir, "pkg-dir", "", "Filesystem path to pkg directory (required)")
	flag.StringVar(&opts.IncludePattern, "include", "", "Regexp for source file paths to include in output")
	flag.StringVar(&opts.IncludePattern, "include-file", "", "Regexp for source file paths to include in output")
	flag.StringVar(&opts.IncludeType, "include-type", "", "Regexp for exported type names to include in output")
	flag.BoolVar(&opts.StripPrefix, "strip-prefix", false, "Remove package prefixes from generated identifiers")
	flag.BoolVar(&opts.DisableRename, "disable-rename", false, "Skip rename scan (TypeNameMapper ignored)")
	defaultOut := typegen.DefaultOutputPath()
	flag.StringVar(&outputPath, "out", defaultOut, "Output file path (defaults to index.d.ts next to the executable)")
	flag.StringVar(&outputPath, "out-file", defaultOut, "Output file path (alias of -out)")
	flag.BoolVar(&toStdout, "stdout", false, "Write output to stdout instead of a file")
	flag.Parse()

	if err := typegen.GenerateTypesToOutput(opts, typegen.OutputOptions{
		OutputPath: outputPath,
		Stdout:     toStdout,
	}); err != nil {
		log.Fatalf("generate types: %v", err)
	}
}
