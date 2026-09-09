package aliyunenterprise

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/hash"
)

// Object implements fs.Object for Aliyun Drive Enterprise.
type Object struct {
	fs         *Fs
	meta       *FileMeta
	remote     string // path relative to the Fs root
	parentPath string
}

var (
	_ fs.Object = (*Object)(nil)
	_ fs.IDer   = (*Object)(nil)
)

// Fs returns the parent Fs.
func (o *Object) Fs() fs.Info { return o.fs }

// String returns a unique description of the object (the remote path, per
// the rclone Object.String() convention). Nil-safe for the fstest NilObject.
func (o *Object) String() string {
	if o == nil {
		return "<nil>"
	}
	return o.remote
}

// Remote returns the remote path relative to the Fs root.
func (o *Object) Remote() string { return o.remote }

// ID returns the provider file_id (stable identity).
func (o *Object) ID() string { return o.meta.FileID }

// ModTime returns the last modified time from the provider.
func (o *Object) ModTime(ctx context.Context) time.Time { return o.meta.ModTime() }

// Size returns the object size in bytes.
func (o *Object) Size() int64 { return o.meta.Size }

// Storable reports that the object can be stored.
func (o *Object) Storable() bool { return true }

// Hash returns the provider SHA-1 (content_hash). Never blocks on download.
func (o *Object) Hash(ctx context.Context, ty hash.Type) (string, error) {
	if ty != hash.SHA1 {
		return "", hash.ErrUnsupported
	}
	if o.meta.ContentHash != "" {
		return o.meta.ContentHash, nil
	}
	m, err := o.fs.remote.client.GetFile(ctx, o.meta.FileID)
	if err != nil {
		return "", err
	}
	o.meta.ContentHash = m.ContentHash
	return o.meta.ContentHash, nil
}

// SetModTime is not supported by the provider; checksum is the source of truth.
func (o *Object) SetModTime(ctx context.Context, t time.Time) error {
	return fs.ErrorCantSetModTime
}

// Open downloads the object, honoring Range/Seek options.
// The PDS CDN ignores Range headers (returns full content), so slicing is done
// client-side (full download + discard). Documented in provider-quirks.md.
func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	start, end := openOptionRange(options, o.meta.Size)
	if start == -1 {
		// no range requested: plain full download
		rc, _, err := o.fs.remote.client.Download(ctx, o.meta.FileID, "")
		return rc, err
	}
	if start >= o.meta.Size {
		return io.NopCloser(strings.NewReader("")), nil
	}
	rc, _, err := o.fs.remote.client.Download(ctx, o.meta.FileID, "")
	if err != nil {
		return nil, err
	}
	r := io.NewSectionReader(newReaderAt(rc), start, end-start+1)
	return &readCloser{r: r, src: rc}, nil
}

// Update overwrites the object content in place (same file_id, new revision).
func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	o2, err := o.fs.remote.putObject(ctx, o.remote, in, src, o.fs)
	if err != nil {
		return err
	}
	if no, ok := o2.(*Object); ok {
		*o = *no
	}
	return nil
}

// Remove implements logical delete: move into the provider-private trash.
func (o *Object) Remove(ctx context.Context) error {
	return o.fs.remote.removeLogical(ctx, o.meta)
}

// ---------------------------------------------------------------- helpers

// openOptionRange resolves the requested byte range from rclone OpenOptions,
// normalized against the object size. Returns (-1,-1) when no range is set.
// rclone RangeOption semantics: Start<0 with End>=0 means the last End bytes
// (suffix range).
func openOptionRange(options []fs.OpenOption, size int64) (int64, int64) {
	var start, end int64 = -1, -1
	for _, o := range options {
		switch x := o.(type) {
		case *fs.SeekOption:
			start, end = x.Offset, size-1
		case *fs.RangeOption:
			if x.Start < 0 && x.End >= 0 {
				start, end = size-x.End, size-1 // suffix: last End bytes
			} else {
				start, end = x.Start, x.End
			}
		}
	}
	if start < 0 {
		return -1, -1
	}
	if end < 0 || end >= size {
		end = size - 1
	}
	return start, end
}

// readerAt adapts a ReadCloser to io.ReaderAt by buffering the single stream
// upfront (the CDN range is unavailable, so we read the full body once).
func newReaderAt(rc io.ReadCloser) io.ReaderAt {
	b, _ := io.ReadAll(rc)
	return bytes.NewReader(b)
}

type readCloser struct {
	r   io.Reader
	src io.ReadCloser
}

func (rc *readCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc *readCloser) Close() error               { return rc.src.Close() }
