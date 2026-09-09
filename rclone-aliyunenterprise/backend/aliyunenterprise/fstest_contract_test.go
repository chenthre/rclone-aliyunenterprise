//go:build integration

// Official rclone fstest/fstests contract suite for the aliyunenterprise
// backend (P6.1).
//
// Requires a live Aliyun Drive Enterprise space via env:
//
//	ALIYUN_ENTERPRISE_API_KEY
//	ALIYUN_ENTERPRISE_DOMAIN_ID
//	ALIYUN_ENTERPRISE_DRIVE_ID
//
// Run:
//
//	ALIYUN_ENTERPRISE_API_KEY=... ALIYUN_ENTERPRISE_DOMAIN_ID=... \
//	ALIYUN_ENTERPRISE_DRIVE_ID=... \
//	go test -tags integration -v -run TestContract -timeout 45m ./backend/aliyunenterprise/
//
// Results are recorded (with per-case classification) in docs/fstest-results.md.
package aliyunenterprise

import (
	"os"
	"testing"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fstest/fstests"
)

// TestContract runs the full rclone Fs/Object contract suite against the
// live remote (pinned rclone v1.75.1). The backend's logical-delete Purge is
// used for cleanup: no real deletion is attempted.
func TestContract(t *testing.T) {
	if os.Getenv("ALIYUN_ENTERPRISE_API_KEY") == "" {
		t.Skip("ALIYUN_ENTERPRISE_API_KEY not set — skipping live contract suite")
	}
	fstests.Run(t, &fstests.Opt{
		RemoteName: ":aliyunenterprise:",
		NilObject:  (*Object)(nil),
		// capabilities this backend honestly does not provide:
		UnimplementableObjectMethods: []string{
			"SetModTime", // provider cannot set modification times
		},
		// the provider normalizes/rejects invalid UTF-8 file names
		SkipInvalidUTF8: true,
	})
}

var _ fs.Object = (*Object)(nil)