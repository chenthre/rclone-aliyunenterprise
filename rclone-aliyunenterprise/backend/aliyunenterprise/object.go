package aliyunenterprise

import (
	"context"
	"fmt"
	"io"
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

// String returns a unique description of the object.
func (o *Object) String() string { return "aliyunenterprise:" + o.remote }

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
func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	rangeHdr := openOptionsToRange(options)
	rc, _, err := o.fs.remote.client.Download(ctx, o.meta.FileID, rangeHdr)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

// Update overwrites the object content in place (same file_id, new revision).
func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	full := o.fs.join(o.remote)
	o2, err := o.fs.remote.putObject(ctx, full, in, src, o.fs)
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

// openOptionsToRange renders rclone OpenOptions as an HTTP Range header.
func openOptionsToRange(options []fs.OpenOption) string {
	var start, end int64 = -1, -1
	var have bool
	for _, o := range options {
		switch x := o.(type) {
		case *fs.SeekOption:
			start, end = x.Offset, -1
			have = true
		case *fs.RangeOption:
			start, end = x.Start, x.End
			have = true
		}
	}
	if !have {
		return ""
	}
	if end >= 0 {
		return fmt.Sprintf("bytes=%d-%d", start, end)
	}
	return fmt.Sprintf("bytes=%d-", start)
}