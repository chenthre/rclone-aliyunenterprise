//go:build integration

// Replicates the fstests FsCopy scenario: server-side copy of a nested file
// into its own directory with an auto-renamed copy name.
package aliyunenterprise

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDiagCopySameDir(t *testing.T) {
	key := os.Getenv("ALIYUN_ENTERPRISE_API_KEY")
	if key == "" {
		t.Skip("env missing")
	}
	f := diagFs2(t, "_ae_diag_cp")
	ctx := context.Background()

	remote := `hello? sausage/êé/Hello, 世界/ " ' @ < > & ? + ≠/z.txt`
	obj, err := f.Put(ctx, strings.NewReader(strings.Repeat("z", 100)), newFakeInfo(remote, 100))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	target := remote + "-copy"
	cp, err := f.Copy(ctx, obj, target)
	if err != nil {
		t.Fatalf("Copy same-dir: %v", err)
	}
	t.Logf("copy ok: remote=%q", cp.Remote())
	time.Sleep(800 * time.Millisecond)

	entries, err := f.List(ctx, `hello? sausage/êé/Hello, 世界/ " ' @ < > & ? + ≠`)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range entries {
		t.Logf("  entry %v", e.Remote())
	}
}