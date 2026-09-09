package aliyunenterprise

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// joinSlash joins a dir path and a name with "/".
func joinSlash(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// dirOf returns the dir part of "a/b/c" → "a/b".
func dirOf(relPath string) string {
	dir, _ := pathSplit(relPath)
	return strings.Trim(dir, "/")
}

// baseOf returns the leaf name of "a/b/c" → "c".
func baseOf(relPath string) string {
	_, f := pathSplit(relPath)
	return f
}

// partChunkSize infers the part size for a multipart upload.
func partChunkSize(parts []PartInfo, size int64) int64 {
	if len(parts) == 0 {
		return size
	}
	if parts[0].PartSize > 0 {
		return parts[0].PartSize
	}
	if size == 0 {
		return 0
	}
	return (size + int64(len(parts)) - 1) / int64(len(parts))
}

// completeParts renders the part_info_list required by /v2/file/complete.
func completeParts(parts []PartInfo, size int64) []PartInfo {
	chunk := partChunkSize(parts, size)
	out := make([]PartInfo, len(parts))
	for i, p := range parts {
		out[i] = PartInfo{
			PartNumber: p.PartNumber,
			PartSize:   chunkSizeFor(p.PartNumber, chunk, size),
		}
	}
	return out
}

func chunkSizeFor(partNumber int, chunk, size int64) int64 {
	start := int64(partNumber-1) * chunk
	if start >= size {
		return 0
	}
	if start+chunk > size {
		return size - start
	}
	return chunk
}

// spoolAndHash copies in to a temp file while computing SHA-1. Returns the
// temp file (seekable for part upload), size and lowercase hex sha1.
func (r *RemoteFs) spoolAndHash(ctx context.Context, in io.Reader) (*os.File, int64, string, error) {
	tmp, err := os.CreateTemp("", "aliyunenterprise-upload-*")
	if err != nil {
		return nil, 0, "", err
	}
	h := sha1.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), in)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, "", err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, "", err
	}
	return tmp, n, hex.EncodeToString(h.Sum(nil)), nil
}
