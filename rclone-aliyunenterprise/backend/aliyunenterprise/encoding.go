package aliyunenterprise

import "strings"

// Name encoding (P7.1): rclone talks to us in logical (Standard-encoded)
// paths. Aliyun Enterprise rejects '/' and '\\' inside a single name
// (400 InvalidParameter.Name), so the backend maps logical names to
// provider-safe physical names via the standard rclone encoder
// (option `encoding`, default Standard|EncodeBackSlash).
//
// Round-trip invariant (per segment):
//
//	FromStandardName(logical) → provider name (encode `/`→`／`, `\`→`＼`, …)
//	ToStandardName(provider)  → logical name

// encPath converts a logical (Standard-encoded, '/'-separated) path into the
// provider physical path, encoding each segment.
func (r *RemoteFs) encPath(logical string) string {
	if r.opt.Enc == 0 || logical == "" {
		return logical
	}
	segs := strings.Split(logical, "/")
	for i := range segs {
		segs[i] = r.opt.Enc.FromStandardName(segs[i])
	}
	return strings.Join(segs, "/")
}

// decPath converts a provider physical path back into a logical path.
func (r *RemoteFs) decPath(provider string) string {
	if r.opt.Enc == 0 || provider == "" {
		return provider
	}
	segs := strings.Split(provider, "/")
	for i := range segs {
		segs[i] = r.opt.Enc.ToStandardName(segs[i])
	}
	return strings.Join(segs, "/")
}

// encName encodes a single logical name into a provider physical name.
func (r *RemoteFs) encName(logical string) string {
	if r.opt.Enc == 0 {
		return logical
	}
	return r.opt.Enc.FromStandardName(logical)
}

// decName decodes a single provider physical name into a logical name.
func (r *RemoteFs) decName(provider string) string {
	if r.opt.Enc == 0 {
		return provider
	}
	return r.opt.Enc.ToStandardName(provider)
}
