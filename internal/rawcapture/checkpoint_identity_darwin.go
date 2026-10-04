package rawcapture

import (
	"fmt"
	"os"
	"syscall"

	"go.kenn.io/agentsview/internal/volumeid"
)

// Only persisted identity needs to survive a reboot. Live capture checks use
// the raw device and inode, which cannot change when the volume cache refreshes.
func checkpointFileIdentity(_ *os.File, info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d", volumeid.Stable(stat.Dev), stat.Ino)
}
