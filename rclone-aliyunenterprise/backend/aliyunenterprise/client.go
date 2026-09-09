package aliyunenterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal REST client for the Aliyun Drive Enterprise data plane.
//
//	base: https://<domain_id>.api.aliyunfile.com
//	auth: Authorization: Bearer <api key>
type Client struct {
	base    string
	driveID string
	apiKey  string
	http    *http.Client
}

const (
	defaultPageSize = 100
	userAgent       = "rclone-aliyunenterprise/aliyunenterprise"
)

// NewClient builds a client for the enterprise domain.
func NewClient(domainID, apiKey, driveID string) (*Client, error) {
	if domainID == "" || apiKey == "" || driveID == "" {
		return nil, fmt.Errorf("domain_id, api_key and drive_id are all required")
	}
	base := fmt.Sprintf("https://%s.api.aliyunfile.com", domainID)
	return &Client{
		base:    base,
		driveID: driveID,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// postJSON performs a JSON POST to a /v2 path.
func (c *Client) postJSON(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal body: %v", ErrProtocol, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrTransient, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var perr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &perr)
		return nil, classifyHTTP(resp.StatusCode, perr.Code, perr.Message)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		// some endpoints may return empty/plain bodies; treat as empty object
		if len(bytes.TrimSpace(raw)) == 0 {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("%w: decode response: %v", ErrProtocol, err)
	}
	return out, nil
}

// isRapidUpload reports whether create returns a rapid (instant) upload.
func isRapidUpload(d map[string]interface{}) bool {
	return d != nil && d["is_rapid"] == true
}

// ------------------------------------------------------------ metadata

// GetFile returns authoritative metadata for file_id.
func (c *Client) GetFile(ctx context.Context, fileID string) (*FileMeta, error) {
	d, err := c.postJSON(ctx, "/v2/file/get", map[string]interface{}{
		"drive_id": c.driveID,
		"file_id":  fileID,
	})
	if err != nil {
		return nil, err
	}
	return metaFromMap(d), nil
}

// ------------------------------------------------------------ listing

// Page holds one page of a paginated listing.
type Page struct {
	Items      []FileMeta
	NextMarker string
	TotalCount int64
}

// listPage calls file/list once.
func (c *Client) listPage(ctx context.Context, parentFileID, marker string, limit int) (*Page, error) {
	body := map[string]interface{}{
		"drive_id":       c.driveID,
		"parent_file_id": parentFileID,
		"limit":          limit,
	}
	if marker != "" {
		body["marker"] = marker
	}
	d, err := c.postJSON(ctx, "/v2/file/list", body)
	if err != nil {
		return nil, err
	}
	return pageFromMap(d), nil
}

// searchPage calls file/search once with a parent_file_id exact match.
func (c *Client) searchPage(ctx context.Context, parentFileID, marker string, limit int) (*Page, error) {
	escaped := strings.ReplaceAll(parentFileID, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	body := map[string]interface{}{
		"drive_id":           c.driveID,
		"query":              fmt.Sprintf(`parent_file_id = "%s"`, escaped),
		"recursive":          false,
		"return_total_count": true,
		"limit":              limit,
	}
	if marker != "" {
		body["marker"] = marker
	}
	d, err := c.postJSON(ctx, "/v2/file/search", body)
	if err != nil {
		return nil, err
	}
	return pageFromMap(d), nil
}

// ListAll returns all direct children via native file/list.
func (c *Client) ListAll(ctx context.Context, parentFileID string) ([]FileMeta, error) {
	return c.paginate(ctx, func(marker string) (*Page, error) {
		return c.listPage(ctx, parentFileID, marker, defaultPageSize)
	})
}

// SearchAll returns all direct children via file/search (fallback).
func (c *Client) SearchAll(ctx context.Context, parentFileID string) ([]FileMeta, error) {
	return c.paginate(ctx, func(marker string) (*Page, error) {
		return c.searchPage(ctx, parentFileID, marker, defaultPageSize)
	})
}

// paginate walks markers, dedupes by file_id, detects marker loops.
func (c *Client) paginate(ctx context.Context, page func(marker string) (*Page, error)) ([]FileMeta, error) {
	var (
		out      []FileMeta
		byID     = map[string]bool{}
		marker   = ""
		seenMark = map[string]bool{}
	)
	for {
		p, err := page(marker)
		if err != nil {
			return nil, err
		}
		for _, m := range p.Items {
			if m.FileID != "" && !byID[m.FileID] {
				byID[m.FileID] = true
				out = append(out, m)
			}
		}
		if p.NextMarker == "" {
			return out, nil
		}
		if seenMark[p.NextMarker] {
			return nil, fmt.Errorf("%w: repeated next_marker %q", ErrConsistency, p.NextMarker)
		}
		seenMark[p.NextMarker] = true
		marker = p.NextMarker
	}
}

// ------------------------------------------------------------ write ops

// CreateFolder creates a folder; checkNameMode in {refuse, ignore, auto_rename}.
func (c *Client) CreateFolder(ctx context.Context, parentFileID, name, checkNameMode string) (*FileMeta, error) {
	d, err := c.postJSON(ctx, "/v2/file/create", map[string]interface{}{
		"drive_id":        c.driveID,
		"parent_file_id":  parentFileID,
		"name":            name,
		"type":            "folder",
		"check_name_mode": checkNameMode,
	})
	if err != nil {
		return nil, err
	}
	return metaFromMap(d), nil
}

// CreateFile starts an upload (or instant rapid upload). Returns meta incl.
// upload_id and part_info_list when a real upload is required.
func (c *Client) CreateFile(ctx context.Context, parentFileID, name string, size int64, sha1, checkNameMode string, fileID string) (*FileMeta, []PartInfo, bool, error) {
	body := map[string]interface{}{
		"drive_id":        c.driveID,
		"parent_file_id":  parentFileID,
		"name":            name,
		"type":            "file",
		"size":            size,
		"check_name_mode": checkNameMode,
	}
	if sha1 != "" {
		body["content_hash"] = strings.ToUpper(sha1)
		body["content_hash_name"] = "sha1"
	}
	if fileID != "" {
		// provide existing file_id to overwrite in place (verified behavior)
		body["file_id"] = fileID
	}
	d, err := c.postJSON(ctx, "/v2/file/create", body)
	if err != nil {
		return nil, nil, false, err
	}
	meta := metaFromMap(d)
	rapid := d["is_rapid"] == true
	var parts []PartInfo
	for _, p := range asSlice(d["part_info_list"]) {
		pm, _ := p.(map[string]interface{})
		parts = append(parts, PartInfo{
			PartNumber: int(parseSize(pm["part_number"])),
			PartSize:   parseSize(pm["part_size"]),
			UploadURL:  strVal(pm["upload_url"]),
		})
	}
	return meta, parts, rapid, nil
}

// UploadPart PUTs bytes to a signed part URL.
func (c *Client) UploadPart(ctx context.Context, url string, r io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, r)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: upload part: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return classifyHTTP(resp.StatusCode, "UploadPart", strings.TrimSpace(string(b)))
	}
	return nil
}

// Complete finishes a multipart upload.
func (c *Client) Complete(ctx context.Context, fileID, uploadID, name, parentFileID string, parts []PartInfo) (*FileMeta, error) {
	var partList []map[string]interface{}
	for _, p := range parts {
		partList = append(partList, map[string]interface{}{
			"part_number": p.PartNumber,
			"part_size":   p.PartSize,
		})
	}
	body := map[string]interface{}{
		"drive_id":       c.driveID,
		"file_id":        fileID,
		"upload_id":      uploadID,
		"name":           name,
		"parent_file_id": parentFileID,
		"part_info_list": partList,
	}
	d, err := c.postJSON(ctx, "/v2/file/complete", body)
	if err != nil {
		return nil, err
	}
	return metaFromMap(d), nil
}

// Move moves/renames an object; returns the new metadata.
// Move moves/renames an object; returns the new metadata.
// The PDS move response is metadata-poor (file_id/file_name/updated_at only),
// so always refresh via file/get for a complete FileMeta.
func (c *Client) Move(ctx context.Context, fileID, toParentFileID, checkNameMode string) (*FileMeta, error) {
	body := map[string]interface{}{
		"drive_id":          c.driveID,
		"file_id":           fileID,
		"to_parent_file_id": toParentFileID,
		"check_name_mode":   checkNameMode,
	}
	if _, err := c.postJSON(ctx, "/v2/file/move", body); err != nil {
		return nil, err
	}
	return c.GetFile(ctx, fileID)
}

// Copy duplicates an object into toParent; returns new metadata.
// Copy duplicates an object into toParent; returns new metadata.
// Refresh via file/get for a complete FileMeta (the copy response is sparse).
func (c *Client) Copy(ctx context.Context, fileID, toParentFileID, checkNameMode string) (*FileMeta, error) {
	body := map[string]interface{}{
		"drive_id":          c.driveID,
		"file_id":           fileID,
		"to_parent_file_id": toParentFileID,
		"check_name_mode":   checkNameMode,
	}
	d, err := c.postJSON(ctx, "/v2/file/copy", body)
	if err != nil {
		return nil, err
	}
	newID := strVal(d["file_id"])
	if newID == "" {
		newID = fileID
	}
	return c.GetFile(ctx, newID)
}

// Update renames or updates metadata of a file.
func (c *Client) Update(ctx context.Context, fileID string, updates map[string]interface{}) (*FileMeta, error) {
	body := map[string]interface{}{
		"drive_id": c.driveID,
		"file_id":  fileID,
	}
	for k, v := range updates {
		body[k] = v
	}
	d, err := c.postJSON(ctx, "/v2/file/update", body)
	if err != nil {
		return nil, err
	}
	return metaFromMap(d), nil
}

// GetDownloadURL returns a signed download URL for file_id.
func (c *Client) GetDownloadURL(ctx context.Context, fileID string) (string, error) {
	d, err := c.postJSON(ctx, "/v2/file/get_download_url", map[string]interface{}{
		"drive_id": c.driveID,
		"file_id":  fileID,
	})
	if err != nil {
		return "", err
	}
	u := strVal(d["url"])
	if u == "" {
		return "", fmt.Errorf("%w: no download url in response", ErrProtocol)
	}
	return u, nil
}

// Download fetches the object bytes (optionally with Range header).
func (c *Client) Download(ctx context.Context, fileID string, rangeHeader string) (io.ReadCloser, int64, error) {
	u, err := c.GetDownloadURL(ctx, fileID)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: download: %v", ErrTransient, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, 0, classifyHTTP(resp.StatusCode, "Download", "")
	}
	return resp.Body, resp.ContentLength, nil
}

// ------------------------------------------------------------ helpers

func metaFromMap(d map[string]interface{}) *FileMeta {
	if d == nil {
		return &FileMeta{}
	}
	return &FileMeta{
		FileID:       strVal(d["file_id"]),
		ParentFileID: strVal(d["parent_file_id"]),
		Name:         strVal(d["name"]),
		Type:         strVal(d["type"]),
		Size:         parseSize(d["size"]),
		ContentHash:  strings.ToLower(strVal(d["content_hash"])),
		HashName:     strVal(d["content_hash_name"]),
		Status:       strVal(d["status"]),
		UploadID:     strVal(d["upload_id"]),
		CreatedAt:    strVal(d["created_at"]),
		UpdatedAt:    strVal(d["updated_at"]),
	}
}

func pageFromMap(d map[string]interface{}) *Page {
	p := &Page{NextMarker: strVal(d["next_marker"])}
	p.TotalCount = parseSize(d["total_count"])
	for _, it := range asSlice(d["items"]) {
		if m, ok := it.(map[string]interface{}); ok {
			p.Items = append(p.Items, *metaFromMap(m))
		}
	}
	return p
}

func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

func strVal(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", s)
	}
}

// PartInfo mirrors part_info_list entries.
type PartInfo struct {
	PartNumber int
	UploadURL  string
	PartSize   int64
	ETag       string
}

var _ = errors.Is
