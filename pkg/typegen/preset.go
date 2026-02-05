package typegen

// Preset bundles common option defaults that can be reused across projects.
type Preset struct {
	IncludePattern string
	IncludeType    string
	StripPrefix    bool
	DisableRename  bool
}

// Options builds an Options value by applying the preset to the provided pkg
// directory and import path.
func (p Preset) Options(pkgDir, pkgPath string) Options {
	return Options{
		PkgDir:         pkgDir,
		PkgPath:        pkgPath,
		IncludePattern: p.IncludePattern,
		IncludeType:    p.IncludeType,
		StripPrefix:    p.StripPrefix,
		DisableRename:  p.DisableRename,
	}
}
