//go:build !darwin

package rawcapture

import "os"

func checkpointFileIdentity(file *os.File, info os.FileInfo) string {
	return stableFileIdentity(file, info)
}
