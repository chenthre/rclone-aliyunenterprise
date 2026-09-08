package aliyunenterprise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempCatalog(t *testing.T) *Catalog {
	t.Helper()
	dir := t.TempDir()
	return NewCatalog(filepath.Join(dir, "catalog.json"))
}

func TestCatalogFreshIdentityAndRoundTrip(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	c.Keep("101", "root", FileMeta{FileID: "f1", Name: "a.md", ParentFileID: "root"})
	c.Flush()

	// reload from disk
	c2 := NewCatalog(c.path)
	if err := c2.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatalf("rebind same identity: %v", err)
	}
	snap := c2.Snapshot("101", "root")
	if len(snap) != 1 || snap[0].FileID != "f1" || snap[0].Name != "a.md" {
		t.Fatalf("round-trip snapshot mismatch: %+v", snap)
	}
}

func TestWrongDriveRejected(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatal(err)
	}
	c.Keep("101", "root", FileMeta{FileID: "f1", Name: "a.md"})
	c.Flush()

	c2 := NewCatalog(c.path)
	err := c2.VerifyIdentity("aliyunenterprise", "bj37789", "999", "")
	if err == nil {
		t.Fatal("expected wrong-drive catalog rejection")
	}
	if !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestCorruptCatalogFailsClosed(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatal(err)
	}
	c.Keep("101", "root", FileMeta{FileID: "f1", Name: "a.md"})
	c.Flush()

	// corrupt the primary file (and remove the .bak)
	if err := os.WriteFile(c.path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(c.path + ".bak")

	c2 := NewCatalog(c.path)
	err := c2.VerifyIdentity("aliyunenterprise", "bj37789", "101", "")
	if err == nil {
		t.Fatal("expected corrupt-catalog failure")
	}
	if !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestBackupRestoresCorruptPrimary(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatal(err)
	}
	c.Keep("101", "root", FileMeta{FileID: "f1", Name: "a.md"})
	c.Flush()

	// corrupt primary only; .bak should exist
	if err := os.WriteFile(c.path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.path + ".bak"); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	c2 := NewCatalog(c.path)
	if err := c2.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatalf("backup restore failed: %v", err)
	}
	snap := c2.Snapshot("101", "root")
	if len(snap) != 1 || snap[0].FileID != "f1" {
		t.Fatalf("backup snapshot mismatch: %+v", snap)
	}
}

func TestNoTmpLeftAfterSave(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatal(err)
	}
	c.Keep("101", "root", FileMeta{FileID: "f1", Name: "a.md"})
	c.Flush()
	if _, err := os.Stat(c.path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file should not remain, err=%v", err)
	}
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	c := tempCatalog(t)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatal(err)
	}
	c.Flush()
	// bump schema version manually
	data, err := os.ReadFile(c.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path, []byte(strings.Replace(string(data), `"version":2`, `"version":3`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(c.path + ".bak")
	c2 := NewCatalog(c.path)
	if err := c2.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err == nil {
		t.Fatal("expected unsupported schema rejection")
	}
}