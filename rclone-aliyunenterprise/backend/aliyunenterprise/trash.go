package aliyunenterprise

import (
	"context"

	"github.com/rclone/rclone/fs"
)

// ensureTrash resolves (creating as needed) the provider-private trash folder
// at the drive root. Its content is never exposed by List (hidden by name at
// root and by parent id otherwise).
func (r *RemoteFs) ensureTrash(ctx context.Context) (string, error) {
	if r.trashDir != "" {
		return r.trashDir, nil
	}
	id, err := r.resolveChildDir(ctx, "root", r.opt.HiddenTrashName)
	if err != nil {
		if !isNotFound(err) {
			return "", err
		}
		m, err := r.client.CreateFolder(ctx, "root", r.opt.HiddenTrashName, "refuse")
		if err != nil {
			return "", err
		}
		id = m.FileID
	}
	r.trashDir = id
	return id, nil
}

// removeLogical implements Object.Remove: move into hidden trash.
func (r *RemoteFs) removeLogical(ctx context.Context, m *FileMeta) error {
	trash, err := r.ensureTrash(ctx)
	if err != nil {
		return err
	}
	if m.ParentFileID == trash {
		return nil // already there
	}
	if _, err := r.client.Move(ctx, m.FileID, trash, "auto_rename"); err != nil {
		return err
	}
	r.catalog.Remove(r.opt.DriveID, m.ParentFileID, m.FileID)
	// Children under a removed folder are cached under paths; drop the subtree.
	r.dropPathSubtree(m.ParentFileID)
	return nil
}

// rmdirLogical implements Fs.Rmdir: move the (empty) folder into hidden trash.
func (r *RemoteFs) rmdirLogical(ctx context.Context, relPath string) error {
	m, err := r.statByPath(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return fs.ErrorDirNotFound
		}
		return err
	}
	if !m.isDir() {
		return fs.ErrorIsFile
	}
	return r.removeLogical(ctx, m)
}

// dropPathSubtree invalidates cached dir ids that live under the given parent.
func (r *RemoteFs) dropPathSubtree(parentID string) {
	// path cache maps path->id; simplest safe approach: clear the whole cache.
	// Cost is negligible for a personal vault; correctness first.
	r.paths = newPathCache()
}