//go:build integration

// Integration smoke test against a real Aliyun Drive Enterprise space.
//
// Required env (never committed, never part of unit runs):
//
//	ALIYUN_ENTERPRISE_API_KEY, ALIYUN_ENTERPRISE_DOMAIN_ID, ALIYUN_ENTERPRISE_DRIVE_ID
//
// Run:
//
//	go test -tags integration -v -run TestIntegration -timeout 10m ./backend/aliyunenterprise/
//
// The test uses an isolated remote path (<drive root>/_ae_integration_test)
// and cleans up after itself via logical delete (hidden trash).
package aliyunenterprise

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/hash"
)

func integrationClient(t *testing.T) *Fs {
	t.Helper()
	key := os.Getenv("ALIYUN_ENTERPRISE_API_KEY")
	domain := os.Getenv("ALIYUN_ENTERPRISE_DOMAIN_ID")
	drive := os.Getenv("ALIYUN_ENTERPRISE_DRIVE_ID")
	if key == "" || domain == "" || drive == "" {
		t.Skip("ALIYUN_ENTERPRISE_* env not set — skipping live integration test")
	}
	m := configmap.Simple{
		"api_key":      key,
		"domain_id":    domain,
		"drive_id":     drive,
		"catalog_path": t.TempDir() + "/catalog.json",
	}
	ctx, _ := filter.AddConfig(context.Background())
	f, err := NewFs(ctx, "it", "_ae_integration_test", m)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	return f.(*Fs)
}

// fakeInfo implements fs.ObjectInfo for Put().
type fakeInfo struct{ remote string }

func (fi fakeInfo) Remote() string                                  { return fi.remote }
func (fi fakeInfo) String() string                                  { return fi.remote }
func (fi fakeInfo) Size() int64                                     { return -1 }
func (fi fakeInfo) ModTime(context.Context) time.Time               { return time.Time{} }
func (fi fakeInfo) Fs() fs.Info                                     { return nil }
func (fi fakeInfo) Hash(context.Context, hash.Type) (string, error) { return "", nil }
func (fi fakeInfo) Storable() bool                                  { return true }

func newFakeInfo(remote string, size int64) fs.ObjectInfo {
	return &sizedInfo{fakeInfo: fakeInfo{remote: remote}, size: size}
}

type sizedInfo struct {
	fakeInfo
	size int64
}

func (s *sizedInfo) Size() int64 { return s.size }

func TestIntegrationRoundTripAndLogicalDelete(t *testing.T) {
	f := integrationClient(t)
	ctx := context.Background()

	if err := f.Mkdir(ctx, "dir/sub"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	content := "integration marker " + time.Now().Format(time.RFC3339Nano)
	obj, err := f.Put(ctx, strings.NewReader(content), newFakeInfo("dir/sub/file.txt", int64(len(content))))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	defer func() { _ = obj.Remove(ctx) }()

	entries, err := f.List(ctx, "dir/sub")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found fs.Object
	for _, e := range entries {
		if o, ok := e.(fs.Object); ok && o.Remote() == "dir/sub/file.txt" {
			found = o
		}
	}
	if found == nil {
		t.Fatalf("uploaded file not found in listing; entries=%d", len(entries))
	}

	rc, err := found.Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != content {
		t.Fatalf("content mismatch: got %q want %q", got, content)
	}

	h, err := found.Hash(ctx, hash.SHA1)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if len(h) != 40 {
		t.Fatalf("expected sha1 hex, got %q", h)
	}

	// overwrite in place
	content2 := content + " updated"
	obj2, err := f.Put(ctx, strings.NewReader(content2), newFakeInfo("dir/sub/file.txt", int64(len(content2))))
	if err != nil {
		t.Fatalf("overwrite Put: %v", err)
	}
	rc2, err := obj2.Open(ctx)
	if err != nil {
		t.Fatalf("Open updated: %v", err)
	}
	got2, err := io.ReadAll(rc2)
	_ = rc2.Close()
	if err != nil {
		t.Fatalf("ReadAll updated: %v", err)
	}
	if string(got2) != content2 {
		t.Fatalf("update mismatch: %q", got2)
	}

	// logical delete: Remove → NewObject must be not-found
	if err := obj2.Remove(ctx); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := f.NewObject(ctx, "dir/sub/file.txt"); err != fs.ErrorObjectNotFound {
		t.Fatalf("expected ErrorObjectNotFound after Remove, got %v", err)
	}

	// cleanup dirs (logical rmdir)
	_ = f.Rmdir(ctx, "dir/sub")
	_ = f.Rmdir(ctx, "dir")
}
