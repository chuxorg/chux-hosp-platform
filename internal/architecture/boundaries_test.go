package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const repoImport = "github.com/chuxorg/chux-hosp-platform"
const moduleImports = repoImport + "/internal/modules/"

// TST-PLT-005-02: production imports must preserve ADR-0002 module ownership.
// Runtime architecture tests are intentionally standard-library-only.
func TestProductionModuleImportBoundaries(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate architecture test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	internalDir := filepath.Join(root, "internal")
	pmsSources := 0

	err := filepath.WalkDir(internalDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(internalDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		sourcePackage := "internal/" + filepath.ToSlash(relative)
		if strings.HasPrefix(sourcePackage, "internal/modules/pms/") {
			pmsSources++
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if err := checkImport(sourcePackage, imported); err != nil {
				t.Errorf("%s imports %s: %v", filepath.ToSlash(path), imported, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pmsSources == 0 {
		t.Fatal("PMS module has no production Go source: boundary check would be vacuous")
	}
}

// checkImport checks package ownership, not SQL ownership. Data access will
// additionally require module-owned migration/query tests when #22 adds tables.
func checkImport(source, imported string) error {
	isPlatform := source == "internal/platform" || strings.HasPrefix(source, "internal/platform/")
	if isPlatform && strings.HasPrefix(imported, moduleImports) {
		return fmt.Errorf("platform infrastructure may not import business modules")
	}
	if !strings.HasPrefix(source, "internal/modules/") {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(source, "internal/modules/"), "/")
	owner := parts[0]
	layer := ""
	if len(parts) > 1 {
		layer = parts[1]
	}
	if strings.HasPrefix(imported, repoImport+"/cmd/") {
		return fmt.Errorf("module may not import the executable composition root")
	}
	if layer != "infrastructure" && (imported == "database/sql" || strings.HasPrefix(imported, "github.com/jackc/pgx/")) {
		return fmt.Errorf("database driver belongs in infrastructure, not %s", layer)
	}
	if layer == "domain" && strings.HasPrefix(imported, repoImport+"/internal/platform") &&
		!strings.HasPrefix(imported, repoImport+"/internal/platform/contracts") {
		return fmt.Errorf("domain cannot depend on platform infrastructure")
	}
	if !strings.HasPrefix(imported, moduleImports) {
		return nil
	}
	target := strings.Split(strings.TrimPrefix(imported, moduleImports), "/")
	otherModule := target[0]
	otherLayer := ""
	if len(target) > 1 {
		otherLayer = target[1]
	}
	if owner != otherModule {
		if owner == "pms" {
			return fmt.Errorf("PMS must not depend on optional module %s", otherModule)
		}
		if otherLayer != "contracts" {
			return fmt.Errorf("module %s may import only public contracts from module %s", owner, otherModule)
		}
		return nil
	}
	switch layer {
	case "domain":
		if otherLayer == "api" || otherLayer == "application" || otherLayer == "infrastructure" {
			return fmt.Errorf("domain may not import %s", otherLayer)
		}
	case "application":
		if otherLayer == "api" || otherLayer == "infrastructure" {
			return fmt.Errorf("application may not import %s", otherLayer)
		}
	case "infrastructure":
		if otherLayer == "api" {
			return fmt.Errorf("infrastructure may not import API")
		}
	case "api":
		if otherLayer == "infrastructure" {
			return fmt.Errorf("API may not import concrete infrastructure; compose it at the root")
		}
	}
	return nil
}

func TestImportPolicyRejectsForbiddenDependencies(t *testing.T) {
	cases := []struct {
		name, source, imported string
		forbidden              bool
	}{
		{"PMS does not need Loyalty", "internal/modules/pms/api", moduleImports + "loyalty/contracts", true},
		{"PMS does not need Experiences", "internal/modules/pms/application", moduleImports + "experiences/api", true},
		{"Platform cannot load modules", "internal/platform", moduleImports + "pms/api", true},
		{"Other modules may use PMS contracts", "internal/modules/loyalty/application", moduleImports + "pms/contracts", false},
		{"Cross-module domain import blocked", "internal/modules/loyalty/application", moduleImports + "pms/domain", true},
		{"PMS domain cannot import repository", "internal/modules/pms/domain", moduleImports + "pms/infrastructure", true},
		{"PMS domain cannot import platform DB", "internal/modules/pms/domain", repoImport + "/internal/platform", true},
		{"PMS application may use domain", "internal/modules/pms/application", moduleImports + "pms/domain", false},
		{"PMS API may use application", "internal/modules/pms/api", moduleImports + "pms/application", false},
		{"PMS API must not import repository", "internal/modules/pms/api", moduleImports + "pms/infrastructure", true},
		{"PMS API must not use database driver", "internal/modules/pms/api", "github.com/jackc/pgx/v5/pgxpool", true},
		{"PMS infrastructure can use driver", "internal/modules/pms/infrastructure", "github.com/jackc/pgx/v5/pgxpool", false},
		{"Module cannot import main", "internal/modules/pms/application", repoImport + "/cmd/hospitality", true},
		{"Standard library remains usable", "internal/modules/pms/domain", "context", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkImport(tc.source, tc.imported)
			if (err != nil) != tc.forbidden {
				t.Fatalf("import rule: source=%s import=%s forbidden=%v error=%v", tc.source, tc.imported, tc.forbidden, err)
			}
		})
	}
}
