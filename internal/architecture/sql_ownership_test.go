package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// DML must not query another business module's tables directly. This catches
// literal SQL in module infrastructure; it does not inspect constructed SQL,
// indirect functions or database views. Integration tests enforce real FKs.
var sqlTableReference = regexp.MustCompile(`(?i)\b(?:from|into|update|join)\s+([a-z_][a-z0-9_]*)`)

// Normalize only known SQL syntax/built-ins; retain every business table read.
func ownedSQLReferences(sql string) [][]string {
	sql = regexp.MustCompile(`(?i)\bDO\s+UPDATE\s+SET\b`).ReplaceAllString(sql, "DO SET")
	sql = regexp.MustCompile(`(?i)\bFROM\s+generate_series\s*\(`).ReplaceAllString(sql, "FROM (")
	return sqlTableReference.FindAllStringSubmatch(sql, -1)
}

func TestModuleSQLTableOwnership(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	modules := filepath.Join(root, "internal", "modules")
	checked := 0
	err := filepath.WalkDir(modules, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(modules, filepath.Dir(path))
		if err != nil {
			return err
		}
		segments := strings.Split(filepath.ToSlash(rel), "/")
		if len(segments) < 2 || segments[1] != "infrastructure" {
			return nil
		}
		owner := segments[0]
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			sql, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("%s has an invalid string literal: %v", path, err)
				return true
			}

			for _, ref := range ownedSQLReferences(sql) {
				checked++
				if !strings.HasPrefix(strings.ToLower(ref[1]), owner+"_") {
					t.Errorf("%s: module %s SQL references non-owned table %s", path, owner, ref[1])
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no module SQL table references were checked")
	}
}

func TestSQLTableReferencePattern(t *testing.T) {
	for _, tc := range []struct {
		sql, table string
	}{
		{"SELECT id FROM pms_rooms", "pms_rooms"},
		{"INSERT INTO pms_properties(name) VALUES('test')", "pms_properties"},
		{"UPDATE loyalty_ledger SET points=0", "loyalty_ledger"},
	} {
		refs := ownedSQLReferences(tc.sql)
		if len(refs) != 1 || refs[0][1] != tc.table {
			t.Fatalf("SQL parser rejected %q: %#v", tc.sql, refs)
		}
	}
}

func TestSQLSyntaxDoesNotHideForeignTables(t *testing.T) {
	sql := "INSERT INTO pms_reservation_pools(id) VALUES(1) ON CONFLICT(id) DO UPDATE SET name=(SELECT name FROM loyalty_accounts)"
	refs := ownedSQLReferences(sql)
	if len(refs) != 2 || refs[0][1] != "pms_reservation_pools" || refs[1][1] != "loyalty_accounts" {
		t.Fatalf("hidden table: %v", refs)
	}
	refs = ownedSQLReferences("SELECT * FROM generate_series(0,2) n JOIN pms_rooms r ON r.id=n")
	if len(refs) != 1 || refs[0][1] != "pms_rooms" {
		t.Fatalf("builtin versus table: %v", refs)
	}
	refs = ownedSQLReferences("SELECT * FROM generate_series")
	if len(refs) != 1 {
		t.Fatal("a table named after a builtin must still be checked")
	}
}
