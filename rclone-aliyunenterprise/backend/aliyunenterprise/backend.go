// Package aliyunenterprise implements an out-of-tree rclone backend for
// Aliyun Drive Enterprise (PDS) accessed through the enterprise API Key.
//
// Domain facts (verified against bj37789, 2026-09):
//   - data plane: https://<domain_id>.api.aliyunfile.com/v2/*
//   - auth:       Authorization: Bearer <uk-... api key>
//   - file/list   on subfolders returns items:[] (service defect)
//   - file/search parent_file_id=<parent>, recursive=false enumerates direct
//     children reliably (final-consistent index)
//   - file/get    is authoritative for a known file_id
//   - delete/trash are NOT available to api key (403 CheckRouterAccessFailed)
//     -> Object.Remove() is implemented as move to a provider-private trash
//   - hash:       content_hash is SHA-1
//
// Correctness invariants (see docs/implementation-notes.md):
//   - unknown / inconsistent state must fail closed, never report deletion
//   - search absence for a known object is NOT a confirmed deletion
package aliyunenterprise

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/lib/encoder"
)

// Options for the aliyunenterprise backend.
//
// Keys are read from rclone config; when absent, env fallbacks allow the
// backend to be wired in tests/PoC without interactive config:
//
//	ALIYUN_ENTERPRISE_API_KEY
//	ALIYUN_ENTERPRISE_DOMAIN_ID
//	ALIYUN_ENTERPRISE_DRIVE_ID
type Options struct {
	APIKey   string `config:"api_key"`
	DomainID string `config:"domain_id"`
	DriveID  string `config:"drive_id"`

	// HiddenTrashName is the provider-private folder (relative to root)
	// used to implement logical delete. Never surfaced by List().
	HiddenTrashName string `config:"hidden_trash_name"`

	// CatalogPath is a local JSON file used as a persistent known-object
	// catalog (provider cache only, not business data). Empty means default.
	CatalogPath string `config:"catalog_path"`

	// SearchRetries tune the search fallback before failing closed.
	SearchRetries    int           `config:"search_retries"`
	SearchRetryDelay time.Duration `config:"search_retry_delay"`

	// Enc defines how logical (Standard-encoded) names map to provider-safe
	// physical names. Aliyun rejects '/' and '\\' inside names, so the default
	// adds EncodeBackSlash to the rclone Standard encoding (which already
	// encodes '/', control chars, dot-leading/trailing names).
	Enc encoder.MultiEncoder `config:"encoding"`
}

// Fs represents the Aliyun Drive Enterprise remote.
type Fs struct {
	name     string       // remote name in rclone config
	root     string       // current root "directory" of the Fs ("" = drive root)
	opt      Options      // parsed config
	remote   *RemoteFs    // shared client + state per remote
	features *fs.Features // optional features (populated in NewFs)
}

// RemoteFs holds client + shared per-remote state (catalog, trash cache).
type RemoteFs struct {
	opt      Options
	client   *Client
	catalog  *Catalog
	paths    *pathCache
	trashDir string // cached provider file_id of hidden trash folder
}

// Check interfaces are satisfied
var _ fs.Fs = (*Fs)(nil)
var _ fs.Object = (*Object)(nil)

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "aliyunenterprise",
		Description: "Aliyun Drive Enterprise (PDS) via API Key (out-of-tree)",
		NewFs:       NewFs,
		Options: []fs.Option{{
			Name:       "api_key",
			Help:       "Aliyun Drive Enterprise API Key (uk-...). Env: ALIYUN_ENTERPRISE_API_KEY",
			IsPassword: true,
		}, {
			Name: "domain_id",
			Help: "Enterprise domain id, e.g. bj37789. Env: ALIYUN_ENTERPRISE_DOMAIN_ID",
		}, {
			Name: "drive_id",
			Help: "Drive id inside the domain (e.g. 101 for team space). Env: ALIYUN_ENTERPRISE_DRIVE_ID",
		}, {
			Name:    "hidden_trash_name",
			Help:    "Provider-private folder name used for logical delete (default _aliyunenterprise_rclone_trash)",
			Default: "_aliyunenterprise_rclone_trash",
		}, {
			Name:    "catalog_path",
			Help:    "Local JSON catalog file path for known objects (advanced)",
			Default: "",
		}, {
			Name:    "search_retries",
			Help:    "Number of retries for search fallback before failing closed",
			Default: 3,
		}, {
			Name:    "search_retry_delay",
			Help:    "Delay between search retries (seconds)",
			Default: "2s",
		}, {
			Name:       "encoding",
			Help:       "The encoding for the backend (see rclone docs). The provider rejects '/' and '\\' in names, so the default encodes both.",
			Default:    encoder.Standard | encoder.EncodeBackSlash,
			Advanced:   true,
		}},
	})
}

// NewFs constructs the Fs for root.
func NewFs(ctx context.Context, name, root string, m configmap.Mapper) (fs.Fs, error) {
	opt := new(Options)
	if err := configstruct.Set(m, opt); err != nil {
		return nil, err
	}

	// Env fallbacks (never override explicit config)
	if opt.APIKey == "" {
		opt.APIKey = os.Getenv("ALIYUN_ENTERPRISE_API_KEY")
	}
	if opt.DomainID == "" {
		opt.DomainID = os.Getenv("ALIYUN_ENTERPRISE_DOMAIN_ID")
	}
	if opt.DriveID == "" {
		opt.DriveID = os.Getenv("ALIYUN_ENTERPRISE_DRIVE_ID")
	}
	if opt.APIKey == "" || opt.DomainID == "" || opt.DriveID == "" {
		return nil, fmt.Errorf("aliyunenterprise: api_key, domain_id and drive_id are required (config or env)")
	}
	if opt.HiddenTrashName == "" {
		opt.HiddenTrashName = "_aliyunenterprise_rclone_trash"
	}

	if opt.Enc == 0 {
		opt.Enc = encoder.Standard | encoder.EncodeBackSlash
	}

	client, err := NewClient(opt.DomainID, opt.APIKey, opt.DriveID)
	if err != nil {
		return nil, err
	}
	catalog := NewCatalog(opt.CatalogPath)

	remote := &RemoteFs{opt: *opt, client: client, catalog: catalog, paths: newPathCache()}

	f := &Fs{
		name:   name,
		root:   strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/"),
		opt:    *opt,
		remote: remote,
	}

	// Bind the catalog to this provider space; a wrong/corrupt catalog is
	// fatal (fail closed) rather than silently mis-reconciling. remoteRoot is
	// informational only (sub-mount Fs instances share the drive-level catalog).
	if err := catalog.VerifyIdentity("aliyunenterprise", opt.DomainID, opt.DriveID, ""); err != nil {
		return nil, err
	}

	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
	}).Fill(ctx, f)

	// If root refers to an existing file, return fs.ErrorIsFile per rclone
	// convention so the parent dir Fs is handed out instead.
	if f.root != "" {
		leafDir, leafName := pathSplit(f.root)
		if leafName != "" {
			fi, err := remote.statByPath(ctx, f.root)
			switch {
			case err == nil && fi.isFile():
				f.root = strings.Trim(leafDir, "/")
				return f, fs.ErrorIsFile
			case err == nil && fi.isDir():
				// ok: root exists as a directory
			case err != nil && !isNotFound(err):
				return nil, err
			}
		}
	}
	return f, nil
}

// Name returns the remote name.
func (f *Fs) Name() string { return f.name }

// Root returns the root path as entered by the user.
func (f *Fs) Root() string { return f.root }

// String returns a description of the fs.
func (f *Fs) String() string { return fmt.Sprintf("AliyunEnterprise root '%s'", f.root) }

// Precision returns the modulus of modification times (ms).
// The provider cannot store client-provided modification times, so we declare
// modtimes unsupported (rclone standard: fs.ModTimeNotSupported). Consumers
// should compare size+checksum, not mtime.
func (f *Fs) Precision() time.Duration { return fs.ModTimeNotSupported }

// Features returns the optional feature flags.
func (f *Fs) Features() *fs.Features { return f.features }

// Hashes returns the supported hash set (SHA-1).
func (f *Fs) Hashes() hash.Set { return hash.NewHashSet(hash.SHA1) }

// join makes the full remote-located path (relative to the drive ROOT,
// not to the fs root) for a path inside the Fs.
func (f *Fs) join(remote string) string {
	if f.root == "" {
		return strings.Trim(remote, "/")
	}
	return strings.Trim(f.root+"/"+strings.Trim(remote, "/"), "/")
}

// rootRel denormalizes an (possibly absolute) remote back to a path relative
// to the Fs root, matching the List() convention.
func (f *Fs) rootRel(remote string) string {
	r := strings.Trim(remote, "/")
	if f.root != "" {
		if p, ok := strings.CutPrefix(r, f.root+"/"); ok {
			return p
		}
	}
	return r
}

// ---------------------------------------------------------------- dir ops

// Mkdir creates the directory dir (rclone "." means the Fs root).
// When dir is the Fs root and the Fs maps to a real provider path, the
// provider directory must be created (bisync relies on Mkdir being idempotent).
func (f *Fs) Mkdir(ctx context.Context, dir string) error {
	if dir == "" || dir == "." {
		if f.root == "" {
			return nil // the drive root always exists
		}
		_, err := f.remote.ensureDirPath(ctx, f.root)
		return err
	}
	_, err := f.remote.ensureDirPath(ctx, f.join(dir))
	return err
}

// Rmdir removes the directory dir (it must be empty).
// Implemented as logical remove of the (empty) folder into hidden trash.
func (f *Fs) Rmdir(ctx context.Context, dir string) error {
	if dir == "" || dir == "." || f.root == "." {
		return fs.ErrorDirectoryNotEmpty // never delete the drive / fs root
	}
	return f.remote.rmdirLogical(ctx, f.join(dir))
}

// ---------------------------------------------------------------- file ops

// NewObject creates / fetches the Object for path (may not exist yet).
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	t := f.rootRel(remote)
	fs.Debugf(nil, "aliyunenterprise: NewObject(remote=%q) -> rel=%q f.root=%q", remote, t, f.root)
	return f.remote.newObject(ctx, t, f)
}

// Put transfers in to the remote path (creating or overwriting).
// src.Remote() is relative to the Fs root per the rclone contract.
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	o, err := f.remote.putObject(ctx, src.Remote(), in, src, f)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// ---------------------------------------------------------------- listing

// List the objects and directories in dir into entries.
func (f *Fs) List(ctx context.Context, dir string) (fs.DirEntries, error) {
	return f.remote.listDir(ctx, dir, f)
}

// ------------------------------------------------------------ optional

// Purge implements fs.Purger: remove a directory and all of its contents
// (logical delete = move to provider-private trash).
func (f *Fs) Purge(ctx context.Context, dir string) error {
	if dir == "" || dir == "." {
		if f.root == "" {
			// never allow wiping the whole drive root
			return fs.ErrorCantPurge
		}
		return f.remote.rmdirLogical(ctx, f.root)
	}
	return f.remote.rmdirLogical(ctx, f.join(dir))
}

// Copy implements server-side copy (verified provider capability).
// remote is relative to the Fs root per the rclone contract. PDS copy takes
// only a target parent, so a target rename is applied afterwards.
func (f *Fs) Copy(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	srcO, ok := src.(*Object)
	if !ok {
		return nil, fs.ErrorCantCopy
	}
	fs.Debugf(src, "Copy -> %q (src name=%q parent=%s)", remote, srcO.meta.Name, srcO.meta.ParentFileID[:12])
	absPath := f.join(remote)
	// The provider refuses server-side copy into the same parent directory
	// (verified: 403 ForbiddenNoPermission.File). Fall back to rclone's generic
	// copy (read + put), which is semantically correct.
	if dirOf(absPath) == f.join(srcO.parentPath) {
		fs.Debugf(src, "same-directory copy not supported server-side; falling back to generic copy")
		return nil, fs.ErrorCantCopy
	}
	if dirOf(absPath) != f.join(srcO.parentPath) {
		fs.Debugf(src, "copy dirs differ: dirOf=%q srcParent=%q (srcO.parentPath=%q)", dirOf(absPath), f.join(srcO.parentPath), srcO.parentPath)
	}
	parentID, err := f.remote.ensureDirPath(ctx, dirOf(absPath))
	if err != nil {
		return nil, err
	}
	newNameLogical := baseOf(absPath)
	newName := f.remote.encName(newNameLogical)
	meta, err := f.remote.client.Copy(ctx, srcO.meta.FileID, parentID, "auto_rename")
	if err != nil {
		return nil, err
	}
	if newNameLogical != "" && newName != srcO.meta.Name {
		if meta, err = f.remote.client.Update(ctx, meta.FileID, map[string]interface{}{"name": newName}); err != nil {
			return nil, err
		}
	}
	f.remote.catalog.Keep(f.remote.opt.DriveID, parentID, *meta)
	return &Object{
		fs:         f,
		meta:       meta,
		remote:     remote,
		parentPath: dirOf(remote),
	}, nil
}

// Move implements server-side move (verified provider capability; also our
// logical-delete primitive). PDS move takes only a target parent; a target
// rename is applied afterwards (a same-directory move is a pure rename).
func (f *Fs) Move(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	srcO, ok := src.(*Object)
	if !ok {
		return nil, fs.ErrorCantMove
	}
	fs.Debugf(src, "Move -> %q (src name=%q parent=%s remote=%q)", remote, srcO.meta.Name, srcO.meta.ParentFileID[:12], srcO.remote)

	// Decide based on the authoritative provider state, not on possibly stale
	// Object metadata (rclone may pass an Object whose meta predates earlier
	// moves/renames).
	cur, err := f.remote.client.GetFile(ctx, srcO.meta.FileID)
	if err != nil {
		return nil, err
	}
	newNameLogical := baseOf(remote)
	newName := f.remote.encName(newNameLogical)
	absLogical := f.join(remote)
	targetParentID, err := f.remote.ensureDirPath(ctx, dirOf(absLogical))
	if err != nil {
		return nil, err
	}

	var meta *FileMeta
	switch {
	case cur.ParentFileID == targetParentID && newName == cur.Name:
		// no-op (same path): keep authoritative metadata
		meta = cur
	case cur.ParentFileID == targetParentID:
		// pure rename in the same directory
		m, err := f.remote.client.Update(ctx, cur.FileID, map[string]interface{}{"name": newName})
		if err != nil {
			return nil, err
		}
		meta = m
	default:
		m, err := f.remote.client.Move(ctx, cur.FileID, targetParentID, "auto_rename")
		if err != nil {
			return nil, err
		}
		if newName != "" && newName != cur.Name {
			if m, err = f.remote.client.Update(ctx, m.FileID, map[string]interface{}{"name": newName}); err != nil {
				return nil, err
			}
		}
		meta = m
	}

	f.remote.catalog.Remove(f.remote.opt.DriveID, cur.ParentFileID, cur.FileID)
	f.remote.catalog.Keep(f.remote.opt.DriveID, meta.ParentFileID, *meta)
	f.remote.paths = newPathCache()
	return &Object{
		fs:         f,
		meta:       meta,
		remote:     remote,
		parentPath: dirOf(remote),
	}, nil
}
