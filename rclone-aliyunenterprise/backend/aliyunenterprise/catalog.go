package aliyunenterprise

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Catalog is a persistent local snapshot of provider directory listings
// (provider cache only — never treated as authoritative truth by itself).
//
// Key:   "driveID|" + parent_file_id  ("root" == drive root)
// Value: last known direct children of that parent.
//
// Its only role in this backend: when file/search (eventual-consistent index)
// misses an object that the catalog knows about, the backend calls file/get on
// the known file_id instead of reporting the object as deleted.
type Catalog struct {
	mu   sync.Mutex
	path string
	dirs map[string][]FileMeta
}

type catalogFile struct {
	Version int                    `json:"version"`
	Dirs    map[string][]FileMeta `json:"dirs"`
}

// NewCatalog loads (or prepares) the catalog file. Empty path uses a default.
func NewCatalog(path string) *Catalog {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		path = filepath.Join(home, ".cache", "rclone-aliyunenterprise", "catalog.json")
	}
	return &Catalog{path: path, dirs: map[string][]FileMeta{}}
}

func (c *Catalog) key(driveID, parentFileID string) string {
	if parentFileID == "" {
		parentFileID = "root"
	}
	return driveID + "|" + parentFileID
}

// Snapshot returns the last known children of parentFileID.
func (c *Catalog) Snapshot(driveID, parentFileID string) []FileMeta {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.dirs[c.key(driveID, parentFileID)]
	cp := make([]FileMeta, len(out))
	copy(cp, out)
	return cp
}

// Update replaces the snapshot for parentFileID.
func (c *Catalog) Update(driveID, parentFileID string, items []FileMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]FileMeta, len(items))
	copy(cp, items)
	c.dirs[c.key(driveID, parentFileID)] = cp
	c.saveLocked()
}

// Keep rotates a single known object into the snapshot (merging by file_id).
func (c *Catalog) Keep(driveID, parentFileID string, m FileMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.key(driveID, parentFileID)
	list := c.dirs[k]
	for i := range list {
		if list[i].FileID == m.FileID {
			list[i] = m
			c.saveLocked()
			return
		}
	}
	list = append(list, m)
	c.dirs[k] = list
	c.saveLocked()
}

// Remove drops an object (authoritative disappearance) from the snapshot.
func (c *Catalog) Remove(driveID, parentFileID, fileID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.key(driveID, parentFileID)
	list := c.dirs[k]
	n := 0
	for _, m := range list {
		if m.FileID != fileID {
			list[n] = m
			n++
		}
	}
	c.dirs[k] = list[:n]
	c.saveLocked()
}

func (c *Catalog) saveLocked() {
	if c.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return
	}
	cf := catalogFile{Version: 1, Dirs: c.dirs}
	data, err := json.Marshal(&cf)
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}

func (c *Catalog) load() {
	if c.path == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var cf catalogFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return
	}
	if cf.Version == 1 && cf.Dirs != nil {
		c.dirs = cf.Dirs
	}
}

// Flush persists explicit state (used for test determinism).
func (c *Catalog) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saveLocked()
}