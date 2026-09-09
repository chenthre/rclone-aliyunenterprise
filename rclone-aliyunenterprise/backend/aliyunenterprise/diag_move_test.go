//go:build integration

// Replicates the fstests FsMove sequence to observe the remote state after
// each move/rename, root-causing the "hello? sausage/.../z.txt leftover"
// listing discrepancy.
package aliyunenterprise

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/filter"
)

func diagFs2(t *testing.T, root string) *Fs {
	t.Helper()
	key := os.Getenv("ALIYUN_ENTERPRISE_API_KEY")
	domain := os.Getenv("ALIYUN_ENTERPRISE_DOMAIN_ID")
	drive := os.Getenv("ALIYUN_ENTERPRISE_DRIVE_ID")
	if key == "" || domain == "" || drive == "" {
		t.Skip("env missing")
	}
	m := configmap.Simple{
		"api_key":      key,
		"domain_id":    domain,
		"drive_id":     drive,
		"catalog_path": t.TempDir() + "/catalog.json",
	}
	ctx, _ := filter.AddConfig(context.Background())
	f, err := NewFs(ctx, "diag", root, m)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	return f.(*Fs)
}

func dumpTree(t *testing.T, f *Fs, ctx context.Context, dir string, depth int) {
	if depth > 6 {
		return
	}
	entries, err := f.List(ctx, dir)
	if err != nil {
		t.Logf("%*sList(%q) err: %v", depth*2, "", dir, err)
		return
	}
	for _, e := range entries {
		switch x := e.(type) {
		case fs.Object:
			t.Logf("%*sFILE %q size=%d", depth*2, "", x.Remote(), x.Size())
		case fs.Directory:
			t.Logf("%*sDIR %q", depth*2, "", x.Remote())
			dumpTree(t, f, ctx, x.Remote(), depth+1)
		}
	}
}

func TestDiagFstestMoveSequence(t *testing.T) {
	f := diagFs2(t, "_ae_diag_mv2")
	ctx := context.Background()
	file1Path := "file name.txt"
	file2Path := `hello? sausage/êé/Hello, 世界/ " ' @ < > & ? + ≠/z.txt`

	put := func(remote, content string) *Object {
		obj, err := f.Put(ctx, strings.NewReader(content), newFakeInfo(remote, int64(len(content))))
		if err != nil {
			t.Fatalf("Put %q: %v", remote, err)
		}
		return obj.(*Object)
	}
	move := func(src fs.Object, dst string) {
		if _, err := f.Move(ctx, src, dst); err != nil {
			t.Fatalf("Move -> %q: %v", dst, err)
		}
		time.Sleep(1500 * time.Millisecond)
	}

	file1 := put(file1Path, strings.Repeat("x", 100))
	file2 := put(file2Path, strings.Repeat("y", 100))
	t.Logf("put file1 id=%s file2 id=%s", file1.meta.FileID, file2.meta.FileID)
	time.Sleep(1500 * time.Millisecond)
	t.Log("== initial ==")
	dumpTree(t, f, ctx, "", 0)

	t.Log("== step1: move file2 -> other.txt ==")
	move(file2, "other.txt")
	dumpTree(t, f, ctx, "", 0)
	cur, _ := f.remote.client.GetFile(ctx, file2.meta.FileID)
	t.Logf("  provider: file2 now name=%q parent=%s", cur.Name, cur.ParentFileID)

	t.Log("== step2: move file1 -> moveTest/other.txt ==")
	move(file1, "moveTest/other.txt")
	dumpTree(t, f, ctx, "", 0)
	cur, _ = f.remote.client.GetFile(ctx, file1.meta.FileID)
	t.Logf("  provider: file1 now name=%q parent=%s", cur.Name, cur.ParentFileID)

	t.Log("== step3: move file1 back ==")
	move(file1, file1Path)
	cur, _ = f.remote.client.GetFile(ctx, file1.meta.FileID)
	t.Logf("  provider: file1 now name=%q parent=%s status=%q", cur.Name, cur.ParentFileID, cur.Status)
	dumpTree(t, f, ctx, "", 0)

	t.Log("== step4: move file2 back ==")
	move(file2, file2Path)
	cur, _ = f.remote.client.GetFile(ctx, file2.meta.FileID)
	t.Logf("  provider: file2 now name=%q parent=%s status=%q", cur.Name, cur.ParentFileID, cur.Status)
	dumpTree(t, f, ctx, "", 0)
}
