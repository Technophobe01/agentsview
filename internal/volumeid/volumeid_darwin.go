//go:build darwin

package volumeid

import "golang.org/x/sys/unix"

var volumes volumeCache

func stable(dev uint64) uint64 {
	return volumes.stable(dev, findMount, mountedVolumeUUID)
}

func findMount(dev uint64) (mountedVolume, bool) {
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil || n <= 0 {
		return mountedVolume{}, false
	}
	buf := make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if err != nil {
		return mountedVolume{}, false
	}
	for _, fs := range buf[:n] {
		if uint64(uint32(fs.Fsid.Val[0])) != dev {
			continue
		}
		path := unix.ByteSliceToString(fs.Mntonname[:])
		return mountedVolume{
			path: path,
			from: unix.ByteSliceToString(fs.Mntfromname[:]),
			// Never query a network volume or trigger an automount.
			queryUUID: fs.Flags&unix.MNT_LOCAL != 0 && fs.Flags&unix.MNT_AUTOMOUNTED == 0,
		}, path != ""
	}
	return mountedVolume{}, false
}
