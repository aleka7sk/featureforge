// Package architecture provides standard-library-only helpers for
// inspecting this module's own package import graph, used by the
// build-failing architecture tests in architecture_test.go (FF-012 §12).
// It adds no dependency beyond the standard library: the boundaries it
// checks must not themselves be checked by an imported analysis library.
package architecture

import (
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// CmdPackages returns every Go package under cmd/, each with its direct,
// non-test imports (FF-018 §13). It returns an empty slice, not an error,
// when cmd/ does not exist -- Phase A's architecture guards must pass before
// cmd/featureforge is written (implementation step 1 precedes step 8).
func CmdPackages() ([]PackageInfo, error) {
	root := filepath.Join(ModuleRoot(), "cmd")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
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

// PEOSModulePath is the PEOS SDK's module path. Only
// internal/engineering/peos may import it (AD-005).
const PEOSModulePath = "github.com/aleka7sk/PEOS"

// DriverModulePath is the PostgreSQL driver's module path. Only
// internal/infrastructure/postgres may import it (AD-020).
const DriverModulePath = "github.com/jackc/pgx"

// ImportsPEOS reports whether imports (as produced by TransitiveImports)
// contains any PEOS package.
func ImportsPEOS(imports map[string]bool) bool {
	return importsModule(imports, PEOSModulePath)
}

// ImportsDriver reports whether imports contains any PostgreSQL driver
// package.
func ImportsDriver(imports map[string]bool) bool {
	return importsModule(imports, DriverModulePath)
}

func importsModule(imports map[string]bool, modulePath string) bool {
	for imp := range imports {
		if strings.HasPrefix(imp, modulePath) {
			return true
		}
	}
	return false
}
