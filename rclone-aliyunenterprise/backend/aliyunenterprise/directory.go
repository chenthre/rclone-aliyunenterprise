package aliyunenterprise

import (
	"context"
	"time"

	"github.com/rclone/rclone/fs"
)

// adir implements fs.Directory for folders.
type adir struct {
	f      *Fs
	remote string
	meta   *FileMeta
}

var _ fs.Directory = (*adir)(nil)

// Fs returns the parent Fs (fs.DirEntry requirement).
func (d *adir) Fs() fs.Info { return d.f }

// String returns a unique description of the directory.
func (d *adir) String() string { return "aliyunenterprise dir: " + d.remote }

// Remote returns the remote path relative to the Fs root.
func (d *adir) Remote() string { return d.remote }

// ModTime returns the folder's update time.
func (d *adir) ModTime(ctx context.Context) time.Time { return d.meta.ModTime() }

// Size returns the folder size (0 for folders).
func (d *adir) Size() int64 { return d.meta.Size }

// Items returns the number of items (unknown; 0).
func (d *adir) Items() int64 { return 0 }

// ID returns the provider folder id.
func (d *adir) ID() string { return d.meta.FileID }