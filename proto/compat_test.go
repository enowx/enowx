package proto

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "record the API as it is in testdata/api.txt")

// TestAPIStaysCompatible keeps the protocol backward compatible: every
// method and event name, frame type, and JSON field (with its type) the
// API had, recorded in testdata/api.txt, must still be there as it was.
// Additions are fine; record them with go test ./proto -run API -update.
// See docs/protocol.md.
func TestAPIStaysCompatible(t *testing.T) {
	now := apiLines(t)
	path := filepath.Join("testdata", "api.txt")
	if *update {
		if err := os.WriteFile(path, []byte(strings.Join(now, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: record the API with -update", err)
	}
	was := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range was {
		if !slices.Contains(now, line) {
			t.Errorf("gone or changed, which older apps and daemons rely on: %s", line)
		}
	}
	var added []string
	for _, line := range now {
		if !slices.Contains(was, line) {
			added = append(added, line)
		}
	}
	if len(added) > 0 {
		t.Errorf("new in the API, not recorded yet (go test ./proto -run API -update):\n  %s", strings.Join(added, "\n  "))
	}
}

// apiLines describes the package's wire API, one sorted line per name or
// field: "method session.create", "event session.updated", "frame hello=1",
// "field SessionInfo.id string", ...
func apiLines(t *testing.T) []string {
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					lines = append(lines, constLines(s)...)
				case *ast.TypeSpec:
					if st, ok := s.Type.(*ast.StructType); ok && s.Name.IsExported() {
						lines = append(lines, fieldLines(s.Name.Name, st)...)
					}
				}
			}
		}
	}
	for ty := TypeHello; ty <= TypePart; ty++ {
		lines = append(lines, "frame "+ty.String()+"="+strconv.Itoa(int(ty)))
	}
	slices.Sort(lines)
	return slices.Compact(lines)
}

func constLines(s *ast.ValueSpec) []string {
	var out []string
	for i, name := range s.Names {
		if i >= len(s.Values) {
			continue
		}
		lit, ok := s.Values[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value, _ := strconv.Unquote(lit.Value)
		switch {
		case strings.HasPrefix(name.Name, "Method"):
			out = append(out, "method "+value)
		case strings.HasPrefix(name.Name, "Event"):
			out = append(out, "event "+value)
		case name.IsExported():
			// States, kinds, modes: values the other side matches on.
			out = append(out, "value "+name.Name+"="+value)
		}
	}
	return out
}

func fieldLines(typeName string, st *ast.StructType) []string {
	var out []string
	for _, field := range st.Fields.List {
		ty := exprString(field.Type)
		if len(field.Names) == 0 {
			out = append(out, "embed "+typeName+" "+ty) // its fields come along
			continue
		}
		if field.Tag == nil {
			continue
		}
		tag, _ := strconv.Unquote(field.Tag.Value)
		name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, "field "+typeName+"."+name+" "+ty)
	}
	return out
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.ArrayType:
		return "[]" + exprString(v.Elt)
	case *ast.MapType:
		return "map[" + exprString(v.Key) + "]" + exprString(v.Value)
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "any"
	}
	return "?"
}
