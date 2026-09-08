package aliyunenterprise

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rclone/rclone/fs"
)

// listDir implements Fs.List. relPath is relative to the Fs root ("" == root).
// A missing directory is reported as an empty listing (standard object-store
// semantics), which lets bisync cold-start into a new remote path.
func (r *RemoteFs) listDir(ctx context.Context, relPath string, f *Fs) (fs.DirEntries, error) {
	absPath := f.join(relPath)
	parentID, err := r.resolveDirID(ctx, absPath)
	if err != nil {
		if isNotFound(err) {
			return fs.DirEntries{}, nil
		}
		return nil, err
	}
	children, err := r.childrenOf(ctx, parentID)
	if err != nil {
		return nil, err // fail closed — never return a possibly-empty listing
	}

	entries := make(fs.DirEntries, 0, len(children))
	for i := range children {
		m := children[i]
		// hide the provider-private trash folder from user listings
		if relPath == "" && m.isDir() && m.Name == r.opt.HiddenTrashName {
			continue
		}
		childPath := joinSlash(relPath, m.Name)
		if m.isDir() {
			entries = append(entries, &adir{f: f, remote: childPath, meta: &m})
		} else {
			o := &Object{
				fs:         f,
				meta:       &m,
				remote:     childPath,
				parentPath: relPath,
			}
			entries = append(entries, o)
		}
	}
	return entries, nil
}

// newObject fetches the object at relPath (existing or placeholder-for-new).
// It must return fs.ErrorObjectNotFound when the object does not exist.
func (r *RemoteFs) newObject(ctx context.Context, relPath string, f *Fs) (fs.Object, error) {
	m, err := r.statByPath(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return nil, fs.ErrorObjectNotFound
		}
		return nil, err
	}
	return &Object{
		fs:         f,
		meta:       m,
		remote:     relPath,
		parentPath: dirOf(relPath),
	}, nil
}

// putObject uploads src bytes to relPath (create or overwrite).
func (r *RemoteFs) putObject(ctx context.Context, relPath string, in io.Reader, src fs.ObjectInfo, f *Fs) (fs.Object, error) {
	parentRel := dirOf(relPath)
	name := baseOf(relPath)
	if name == "" {
		return nil, fmt.Errorf("%w: invalid remote path %q", ErrProtocol, relPath)
	}
	parentID, err := r.ensureDirPath(ctx, parentRel)
	if err != nil {
		return nil, err
	}

	// spool to temp file to learn size + SHA1 (provider hash), then stream
	tmp, size, sha1, err := r.spoolAndHash(ctx, in)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()

	// existing object → overwrite in place via file_id
	var existingID string
	if meta, err := r.statByPath(ctx, relPath); err == nil && meta.isFile() {
		existingID = meta.FileID
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}

	meta, parts, rapid, err := r.client.CreateFile(ctx, parentID, name, size, sha1, "ignore", existingID)
	if err != nil {
		return nil, err
	}
	if !rapid {
		chunk := partChunkSize(parts, size)
		for _, p := range parts {
			if _, err := tmp.Seek(0, 0); err != nil {
				return nil, err
			}
			start := int64(p.PartNumber-1) * chunk
			limit := chunk
			if start+limit > size {
				limit = size - start
			}
			if limit < 0 {
				limit = 0
			}
			if err := r.client.UploadPart(ctx, p.UploadURL, io.LimitReader(tmp, limit)); err != nil {
				return nil, err
			}
		}
		completed, err := r.client.Complete(ctx, meta.FileID, meta.UploadID, name, parentID, completeParts(parts, size))
		if err != nil {
			return nil, err
		}
		meta = completed
	}

	r.catalog.Keep(r.opt.DriveID, parentID, *meta)
	return &Object{
		fs:         f,
		meta:       meta,
		remote:     relPath,
		parentPath: parentRel,
	}, nil
}