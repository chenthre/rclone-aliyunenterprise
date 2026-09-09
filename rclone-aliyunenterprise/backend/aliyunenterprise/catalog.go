package aliyunenterprise

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Catalog is a persistent local snapshot of provider directory listings.
// It is NOT a performance cache: it is safety state that lets the backend
// guarantee "search absence != confirmed deletion" (see implementation notes
// §2-§3).
//
// Key:   "driveID|" + parent_file_id  ("root" == drive root)
// Value: last known direct children of that parent.
//
// Durability requirements (P5.2):
//   - atomic write (tmp + fsync + rename) with a .bak fallback
//   - schema version + provider/drive identity
//   - corruption detection: reject wrong/unknown catalogs (fail closed)
type Catalog struct {
	mu      sync.Mutex
	path    string
	dirs    map[string][]FileMeta
	id      identity
	loadErr error // non-nil when the catalog was unreadable (corrupt)
}

type identity struct {
	Provider   string `json:"provider_type"`
	DomainID   string `json:"domain_id"`
	DriveID    string `json:"drive_id"`
	RemoteRoot string `json:"remote_root"`
}

type catalogFile struct {
	Version  int                   `json:"version"`
	Identity identity              `json:"identity"`
	Dirs     map[string][]FileMeta `json:"dirs"`
}

const catalogSchemaVersion = 2

// NewCatalog loads (or prepares) the catalog file. Empty path uses a default.
func NewCatalog(path string) *Catalog {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		path = filepath.Join(home, ".cache", "rclone-aliyunenterprise", "catalog.json")
	}
	c := &Catalog{path: path, dirs: map[string][]FileMeta{}}
	c.load()
	return c
}

// VerifyIdentity binds the catalog to a provider space. Errors are fatal for
// the backend (fail closed): using another drive's catalog would corrupt
// reconciliation.
func (c *Catalog) VerifyIdentity(provider, domainID, driveID, remoteRoot string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return fmt.Errorf("%w: catalog unreadable: %v", ErrCatalog, c.loadErr)
	}
	if c.id.Provider == "" && c.id.DomainID == "" && c.id.DriveID == "" {
		// first bind (fresh or legacy empty catalog)
		c.id = identity{Provider: provider, DomainID: domainID, DriveID: driveID, RemoteRoot: remoteRoot}
		return nil
	}
	if c.id.Provider != provider || c.id.DomainID != domainID || c.id.DriveID != driveID {
		return fmt.Errorf("%w: catalog belongs to %s/%s/%s, not %s/%s/%s",
			ErrCatalog, c.id.Provider, c.id.DomainID, c.id.DriveID,
			provider, domainID, driveID)
	}
	return nil
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

// saveLocked writes atomically with fsync + rename, keeping a .bak copy.
func (c *Catalog) saveLocked() {
	if c.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return
	}
	cf := catalogFile{Version: catalogSchemaVersion, Identity: c.id, Dirs: c.dirs}
	data, err := json.Marshal(&cf)
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	if _, err := f.Write(data); err == nil {
		_ = f.Sync() // fsync before rename
	}
	if err := f.Close(); err != nil {
		return
	}
	// keep previous good version as .bak, then rename (atomic publish)
	if prev, err := os.ReadFile(c.path); err == nil && !errors.Is(err, os.ErrNotExist) && len(prev) > 0 {
		_ = os.WriteFile(c.path+".bak", prev, 0o600)
	}
	if err := os.Rename(tmp, c.path); err == nil {
		_ = os.Remove(tmp)
	}
}

// load reads the catalog; on corruption tries catalog.json.bak.
func (c *Catalog) load() {
	if c.path == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return // fresh — not an error
		}
		c.loadErr = err
		return
	}
	if err := c.parse(data); err == nil {
		return
	} else if backup, berr := os.ReadFile(c.path + ".bak"); berr == nil {
		// prefer the backup over failing completely (corruption detection)
		if err2 := c.parse(backup); err2 == nil {
			return
		}
		c.loadErr = fmt.Errorf("catalog corrupt (primary and backup): %v", err)
		return
	} else {
		c.loadErr = fmt.Errorf("catalog corrupt: %v", err)
		return
	}
}

func (c *Catalog) parse(data []byte) error {
	var cf catalogFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return err
	}
	if cf.Version > catalogSchemaVersion {
		return fmt.Errorf("unsupported schema version %d", cf.Version)
	}
	if cf.Version == 1 {
		// legacy: identity unknown; treat as empty identity (re-bind on use)
		c.id = identity{}
		c.dirs = cf.Dirs
		if c.dirs == nil {
			c.dirs = map[string][]FileMeta{}
		}
		return nil
	}
	if cf.Dirs == nil {
		c.dirs = map[string][]FileMeta{}
	} else {
		c.dirs = cf.Dirs
	}
	c.id = cf.Identity
	return nil
}

// Flush persists explicit state (used for test determinism).
func (c *Catalog) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saveLocked()
}
