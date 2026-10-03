// Package volumeid turns the device number in a file's stat into a value that
// identifies the same volume across reboots.
//
// Sync records a file's inode and device so a replaced file is never appended
// to from a stale offset. That needs the device half to mean "this volume" for
// as long as the archive lives. On Linux st_dev does. On macOS it does not: a
// volume's st_dev is assigned when it is mounted, and after a system update the
// data volume can come back with a different number while every file keeps its
// inode, which made every session look replaced.
package volumeid

// Stable returns a device value for dev that does not change when the volume
// is remounted. On platforms where st_dev is already stable it returns dev.
func Stable(dev uint64) uint64 { return stable(dev) }
