// Package architecture provides standard-library-only helpers for
// inspecting this module's own package import graph, used by the
// build-failing architecture tests in architecture_test.go (FF-012 §12).
// It adds no dependency beyond the standard library (FF-013 §3: PEOS is the
// only module requirement M.3 adds).
package architecture

import (
	"go/build"
	"io/fs"
	"path/filepath"
	"runtime"
)

// ModulePath is this module's import path, matching go.mod.
const ModulePath = "github.com/aleka7sk/featureforge"

// ModuleRoot returns the absolute path to the module root, derived from
// this file's own location so it never depends on the working directory a
// test happens to run from.
func ModuleRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// PackageInfo describes one package's own direct, non-test imports.
type PackageInfo struct {
	ImportPath string
	Dir        string
	Imports    []string
}

// InternalPackages returns every Go package under internal/, each with its
// direct, non-test imports (build.ImportDir's default mode excludes test
// files, matching what production code -- not test helpers -- actually
// imports, which is what the boundary rules govern).
func InternalPackages() ([]PackageInfo, error) {
	root := filepath.Join(ModuleRoot(), "internal")
	var pkgs []PackageInfo
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		pkg, ierr := build.ImportDir(path, 0)
		if ierr != nil {
			if _, ok := ierr.(*build.NoGoError); ok {
				return nil
			}
			return ierr
		}
		rel, rerr := filepath.Rel(ModuleRoot(), path)
		if rerr != nil {
			return rerr
		}
		pkgs = append(pkgs, PackageInfo{
			ImportPath: ModulePath + "/" + filepath.ToSlash(rel),
			Dir:        path,
			Imports:    pkg.Imports,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// TransitiveImports returns every import reachable from importPath by
// following internal-package edges (imports of packages outside this
// module's internal/ tree are recorded but not followed further, since
// their own dependency graphs are not this module's concern). The result
// includes importPath's own direct imports.
func TransitiveImports(importPath string, all []PackageInfo) map[string]bool {
	byPath := make(map[string]PackageInfo, len(all))
	for _, p := range all {
		byPath[p.ImportPath] = p
	}
	seen := map[string]bool{}
	result := map[string]bool{}
	var visit func(string)
	visit = func(ip string) {
		if seen[ip] {
			return
		}
		seen[ip] = true
		pkg, ok := byPath[ip]
		if !ok {
			return
		}
		for _, imp := range pkg.Imports {
			result[imp] = true
			if _, isInternal := byPath[imp]; isInternal {
				visit(imp)
			}
		}
	}
	visit(importPath)
	return result
}

// ImportsPEOS reports whether imports (as produced by TransitiveImports)
// contains any github.com/aleka7sk/PEOS package.
func ImportsPEOS(imports map[string]bool) bool {
	for imp := range imports {
		if len(imp) >= len("github.com/aleka7sk/PEOS") && imp[:len("github.com/aleka7sk/PEOS")] == "github.com/aleka7sk/PEOS" {
			return true
		}
	}
	return false
}
