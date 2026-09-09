# testserver init.d fixture (empty)

rclone's `fstest/testserver.Start` looks for config files in
`fstest/testserver/init.d` relative to the current working directory
(up to 5 levels up) even for remotes that have no start command.

In an out-of-tree build there is no rclone source tree to satisfy this,
so we ship this empty directory. `hasStartCommand` returns false for
every backend name here (`aliyunenterprise` included), and the suite
proceeds with no test server.
