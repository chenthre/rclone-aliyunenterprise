package aliyunenterprise

import (
	"strconv"
	"strings"
	"time"
)

// FileMeta is the minimal provider object metadata we need.
type FileMeta struct {
	FileID       string `json:"file_id"`
	ParentFileID string `json:"parent_file_id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "file" | "folder"
	Size         int64  `json:"size,omitempty"`
	ContentHash  string `json:"content_hash,omitempty"`
	HashName     string `json:"content_hash_name,omitempty"`
	Status       string `json:"status,omitempty"` // "available" | "uploading" ...
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	UploadID     string `json:"upload_id,omitempty"`
	Hidden       bool   `json:"-"` // internal: is under the provider-private trash
}

func (m *FileMeta) isFile() bool { return m.Type == "file" }
func (m *FileMeta) isDir() bool  { return m.Type == "folder" }

// ModTime parses the provider updated_at (ISO8601 with fractional seconds).
func (m *FileMeta) ModTime() time.Time {
	t, err := time.Parse(time.RFC3339, m.UpdatedAt)
	if err != nil {
		// some responses use "+08:00" offsets; RFC3339 covers that.
		if t2, err2 := time.Parse("2006-01-02T15:04:05.000Z07:00", m.UpdatedAt); err2 == nil {
			return t2
		}
		return time.Time{}
	}
	return t
}

// parseSize is a small helper for JSON strings that may arrive as string.
func parseSize(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	case int64:
		return n
	}
	return 0
}

// pathSplit splits "/a/b/c" -> ("/a/b/", "c"); mirrors path.Split behavior.
func pathSplit(p string) (dir, file string) {
	p = strings.Trim(p, "/")
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", p
	}
	return p[:i+1], p[i+1:]
}

var _ = strings.TrimPrefix