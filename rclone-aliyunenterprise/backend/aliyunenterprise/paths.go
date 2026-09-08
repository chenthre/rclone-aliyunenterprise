package aliyunenterprise

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rclone/rclone/fs"
)

// pathCache maps remote-relative paths (string) to their provider file_id.
// Key convention: "" == drive root ("root" parent). Only folder paths are
// cached (files are resolved per-operation and held on Object).
type pathCache struct {
	mu   sync.Mutex
	dirs map[string]string
}

func newPathCache() *pathCache {
	return &pathCache{dirs: map[string]string{"": "root"}}
}

func (pc *pathCache) get(p string) (string, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	id, ok := pc.dirs[p]
	return id, ok
}

func (pc *pathCache) set(p, id string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.dirs[p] = id
}

// segments splits a remote path into clean segments.
func segments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ensureDirPath resolves (creating as needed) every folder on relPath and
// returns its provider file_id. "" → "root" (drive root).
func (r *RemoteFs) ensureDirPath(ctx context.Context, relPath string) (string, error) {
	if relPath == "" || relPath == "." {
		return "root", nil
	}
	if id, ok := r.paths.get(relPath); ok {
		return id, nil
	}
	parts := segments(relPath)
	parentID := "root"
	for i, seg := range parts {
		cur := strings.Join(parts[:i+1], "/")
		if id, ok := r.paths.get(cur); ok {
			parentID = id
			continue
		}
		id, err := r.resolveChildDir(ctx, parentID, seg)
		if err != nil {
			if !isNotFound(err) {
				return "", err
			}
			// create missing folder
			m, err := r.client.CreateFolder(ctx, parentID, seg, "refuse")
			if err != nil {
				return "", err
			}
			id = m.FileID
		}
		r.paths.set(cur, id)
		parentID = id
	}
	return parentID, nil
}

// resolveDirID resolves an existing folder path without creation.
func (r *RemoteFs) resolveDirID(ctx context.Context, relPath string) (string, error) {
	if relPath == "" || relPath == "." {
		return "root", nil
	}
	if id, ok := r.paths.get(relPath); ok {
		return id, nil
	}
	parts := segments(relPath)
	parentID := "root"
	for i, seg := range parts {
		cur := strings.Join(parts[:i+1], "/")
		if id, ok := r.paths.get(cur); ok {
			parentID = id
			continue
		}
		id, err := r.resolveChildDir(ctx, parentID, seg)
		if err != nil {
			return "", err
		}
		r.paths.set(cur, id)
		parentID = id
	}
	return parentID, nil
}

// resolveChildDir finds a folder named name inside parentID via listing.
func (r *RemoteFs) resolveChildDir(ctx context.Context, parentID, name string) (string, error) {
	children, err := r.childrenOf(ctx, parentID)
	if err != nil {
		return "", err
	}
	for _, m := range children {
		if m.isDir() && m.Name == name {
			return m.FileID, nil
		}
	}
	return "", ErrNotFound
}

// statByPath returns the metadata of the object at relPath ("" → root dir).
func (r *RemoteFs) statByPath(ctx context.Context, relPath string) (*FileMeta, error) {
	if relPath == "" || relPath == "." {
		return &FileMeta{FileID: "root", ParentFileID: "", Name: "", Type: "folder"}, nil
	}
	dirPart, leaf := pathSplit(relPath)
	dirPart = strings.Trim(dirPart, "/")
	parentID, err := r.resolveDirID(ctx, dirPart)
	if err != nil {
		return nil, err
	}
	children, err := r.childrenOf(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for _, m := range children {
		if m.Name == leaf {
			if m.Type == "" {
				m.Type = "file"
			}
			return &m, nil
		}
	}
	return nil, ErrNotFound
}

// childrenOf is the merged, verified child enumeration for parentID.
// Implements guide §7.2-7.4: list → search fallback → catalog reconcile.
func (r *RemoteFs) childrenOf(ctx context.Context, parentID string) ([]FileMeta, error) {
	// 1) native list first (authoritative when non-empty / root)
	var items []FileMeta
	listed, err := r.client.ListAll(ctx, parentID)
	if err != nil {
		return nil, err
	}
	items = listed

	// 2) subdir empty → search fallback (with retries)
	if parentID != "root" && len(items) == 0 {
		fs.Debugf(nil, "aliyunenterprise: native list empty for parent %s -> search fallback", parentID)
		searched, err := r.searchWithRetry(ctx, parentID)
		if err != nil {
			return nil, err // fail closed
		}
		// 2b) verify each search hit with file/get (authoritative) and filter
		// by the real parent — the search index is eventual-consistent and can
		// briefly report stale parents after move/delete (guide §7.2).
		items = items[:0]
		verifyErr := 0
		for _, m := range searched {
			cur, err := r.client.GetFile(ctx, m.FileID)
			if err != nil {
				if isNotFound(err) {
					continue // stale index entry
				}
				verifyErr++
				if verifyErr > 5 {
					return nil, err // get cascade → fail closed
				}
				continue
			}
			if cur.ParentFileID == parentID && cur.Status != "uploading" && cur.Status != "" {
				cur.ContentHash = strings.ToLower(cur.ContentHash)
				items = append(items, *cur)
			}
		}
	}

	// 3) reconcile against catalog: search-missed known objects are verified
	//    by file/get, never assumed deleted (guide §7.4 invariant).
	searched := map[string]bool{}
	for _, m := range items {
		searched[m.FileID] = true
	}
	knownErr := 0
	for _, known := range r.catalog.Snapshot(r.opt.DriveID, parentID) {
		if searched[known.FileID] {
			continue
		}
		current, err := r.client.GetFile(ctx, known.FileID)
		if err != nil {
			if isNotFound(err) {
				r.catalog.Remove(r.opt.DriveID, parentID, known.FileID)
				continue
			}
			knownErr++
			if knownErr > 5 {
				return nil, err // get cascade → fail closed
			}
			continue
		}
		if current.ParentFileID == parentID && current.Status != "uploading" && current.Status != "" {
			items = append(items, *current)
			searched[current.FileID] = true
		} else if current.ParentFileID != parentID {
			// object has moved away from this dir — drop it from the snapshot
			r.catalog.Remove(r.opt.DriveID, parentID, known.FileID)
		}
	}

	// 4) refresh catalog snapshot
	r.catalog.Update(r.opt.DriveID, parentID, items)
	return items, nil
}

// searchWithRetry runs the search fallback with bounded retries, then fails
// closed (returns error) if it cannot obtain a trustworthy page.
func (r *RemoteFs) searchWithRetry(ctx context.Context, parentID string) ([]FileMeta, error) {
	var (
		items []FileMeta
		err   error
	)
	for attempt := 0; attempt <= r.opt.SearchRetries; attempt++ {
		items, err = r.client.SearchAll(ctx, parentID)
		if err == nil {
			return items, nil
		}
		if !isRetryable(err) {
			return nil, err
		}
		if attempt < r.opt.SearchRetries {
			sleepCtx(ctx, r.opt.SearchRetryDelay)
		}
	}
	return nil, err
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}