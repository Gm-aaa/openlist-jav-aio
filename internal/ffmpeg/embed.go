package ffmpeg

import "embed"

//go:embed assets/windows_amd64
var windowsAMD64 embed.FS

//go:embed assets/linux_amd64
var linuxAMD64 embed.FS
