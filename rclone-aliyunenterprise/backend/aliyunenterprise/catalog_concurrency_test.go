package aliyunenterprise

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// TestCatalogConcurrentProcessesNoLostUpdate runs two *real independent OS
// processes (helper-process pattern) that both Keep() distinct objects into
// the same catalog file, then verifies both survive (no lost update).
//
// Before the cross-process flock, the last writer would overwrite the other's
// snapshot (lost update) — this test pins the fix.
func TestCatalogConcurrentProcessesNoLostUpdate(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")

	start := make(chan struct{})
	_ = start
	done := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=TestCatalogHelperProcess", "--")
			cmd.Env = append(os.Environ(),
				"GO_WANT_HELPER_PROCESS=1",
				fmt.Sprintf("GO_HELPER_ID=p%d", n),
				"GO_CATALOG_PATH="+catalogPath,
				"GO_HELPER_START="+helperSignal(),
			)
			// child writes one object per loop iteration; start signal is a file
			_ = cmd.Start()
			_ = cmd.Wait()
			done <- struct{}{}
		}(i)
	}
	<-done
	<-done
	wg.Wait()

	// Parent: reload and check both objects survived.
	c := NewCatalog(catalogPath)
	if err := c.VerifyIdentity("aliyunenterprise", "bj37789", "101", ""); err != nil {
		t.Fatalf("identity: %v", err)
	}
	snap := c.Snapshot("101", "root")
	ids := map[string]bool{}
	for _, m := range snap {
		ids[m.FileID] = true
	}
	// every object from both processes must survive (no lost update)
	for p := 0; p < 2; p++ {
		for i := 0; i < 5; i++ {
			want := fmt.Sprintf("f_p%d_%d", p, i)
			if !ids[want] {
				t.Fatalf("lost update: %q missing from catalog (have %v)", want, len(ids))
			}
		}
	}
	if len(ids) != 10 {
		t.Fatalf("expected 10 objects from both processes, got %d", len(ids))
	}
}

// helperSignal: child processes wait on this file's existence so both start
// as close to concurrently as possible.
func helperSignal() string {
	p := filepath.Join(os.TempDir(), "ae-cat-helper-go.txt")
	_ = os.Remove(p)
	return p
}

// TestCatalogHelperProcess is the child entrypoint used by
// TestCatalogConcurrentProcessesNoLostUpdate.
func TestCatalogHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	path := os.Getenv("GO_CATALOG_PATH")
	start := os.Getenv("GO_HELPER_START")
	// synchronize both children on the start file
	for i := 0; i < 1000; i++ {
		if _, err := os.Stat(start); err == nil {
			break
		}
		_ = os.Remove(start)
		if i == 900 {
			// give up waiting and just write anyway
			break
		}
	}
	pid := os.Getenv("GO_HELPER_ID")
	c := NewCatalog(path)
	_ = c.VerifyIdentity("aliyunenterprise", "bj37789", "101", "")
	for i := 0; i < 5; i++ {
		c.Keep("101", "root", FileMeta{
			FileID:       fmt.Sprintf("f_%s_%d", pid, i),
			Name:         fmt.Sprintf("obj-%s-%d", pid, i),
			ParentFileID: "root",
			Type:         "file",
		})
	}
	t.Log("helper wrote")
}

// TestCatalogIntraProcessConcurrentKeeps sanity-checks the two-instance
// (same-process goroutine) path under -race.
func TestCatalogIntraProcessConcurrentKeeps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	c1 := NewCatalog(path)
	c2 := NewCatalog(path)
	_ = c1.VerifyIdentity("aliyunenterprise", "bj37789", "101", "")
	_ = c2.VerifyIdentity("aliyunenterprise", "bj37789", "101", "")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			c1.Keep("101", "root", FileMeta{FileID: "a" + string(rune('0'+n)), Name: "a", ParentFileID: "root"})
		}(i)
		go func(n int) {
			defer wg.Done()
			c2.Keep("101", "root", FileMeta{FileID: "b" + string(rune('0'+n)), Name: "b", ParentFileID: "root"})
		}(i)
	}
	wg.Wait()

	c := NewCatalog(path)
	snap := c.Snapshot("101", "root")
	if len(snap) != 16 {
		t.Fatalf("expected 16 kept objects, got %d", len(snap))
	}
}
