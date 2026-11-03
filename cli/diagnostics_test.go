package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/diag"
	"github.com/enowx/enowx/transport"
)

// It works with no daemon: the logs are what says why there is none.
func TestDiagnosticsWithNoDaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir, err := transport.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, diag.AppLog), []byte("2026-11-04T01:00:00+07:00 conn.closed conn=ab by=daemon why=EOF token=abcd1234efgh5678\n2026-11-04T01:00:01+07:00 page.change to=canvas\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"diagnostics"}, &out); err != nil {
		t.Fatal(err)
	}
	if text := out.String(); !strings.Contains(text, "conn.closed conn=ab") || strings.Contains(text, "page.change") || strings.Contains(text, "abcd1234efgh5678") {
		t.Fatalf("%s", text)
	}
	out.Reset()
	if err := Run([]string{"diagnostics", "--all"}, &out); err != nil || !strings.Contains(out.String(), "page.change") {
		t.Fatalf("%v: %s", err, out.String())
	}
	zipFile := filepath.Join(t.TempDir(), "bundle.zip")
	out.Reset()
	if err := Run([]string{"diagnostics", "--bundle", zipFile}, &out); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(zipFile)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	found := false
	for _, f := range z.File {
		found = found || strings.HasSuffix(f.Name, "/"+diag.AppLog)
	}
	if !found {
		t.Fatal("no log in the bundle")
	}
}
