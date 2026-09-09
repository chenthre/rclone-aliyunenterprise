// Command rclone-aliyunenterprise: rclone with the out-of-tree aliyunenterprise
// backend for Aliyun Drive Enterprise (PDS, enterprise api_key).
//
// Build:
//
//	go build -o rclone-aliyunenterprise ./cmd/rclone-aliyunenterprise
//
// Usage (config via env, key never embedded):
//
//	ALIYUN_ENTERPRISE_API_KEY=uk-... ALIYUN_ENTERPRISE_DOMAIN_ID=bj37789 \
//	ALIYUN_ENTERPRISE_DRIVE_ID=101 rclone-aliyunenterprise lsf :aliyunenterprise:
package main

import (
	"github.com/rclone/rclone/cmd"

	_ "github.com/rclone/rclone/backend/all" // import all the in-tree backends
	_ "github.com/rclone/rclone/cmd/all"     // import all the rclone commands (version, bisync, ...)
	_ "github.com/rclone/rclone/fs/sync"     // import sync functions (bisync etc)

	_ "github.com/chenthre/rclone-aliyunenterprise/backend/aliyunenterprise" // our backend
)

func main() {
	cmd.Main()
}
