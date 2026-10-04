package volumeid

import (
	"hash/fnv"
	"sync"
	"time"
)

const refreshAfter = 30 * time.Second

type uuidAnswer int

const (
	uuidFailed uuidAnswer = iota
	uuidNone
	uuidFound
)

type mountedVolume struct {
	path, from string
	queryUUID  bool
}

type cachedVolume struct {
	mu      sync.Mutex
	mount   mountedVolume
	value   uint64
	expires time.Time
}

// volumeCache queries only requested devices. Entries expire after 30 seconds;
// the next lookup rechecks the mount and UUID, including when a replacement
// reuses the device number and mount path. There is no background work.
// A slow filesystem call blocks lookups of that device, not other devices.
// Failed reads keep the last identity; a first failure uses the mount path
// (or the raw device number if the mount table is unavailable).
type volumeCache struct {
	mu      sync.Mutex
	devices map[uint64]*cachedVolume
}

func (c *volumeCache) stable(
	dev uint64,
	lookup func(uint64) (mountedVolume, bool),
	query func(string) ([16]byte, uuidAnswer),
) uint64 {
	// Darwin dev_t is signed 32-bit; callers may widen it either way.
	key := uint64(uint32(dev))
	c.mu.Lock()
	if c.devices == nil {
		c.devices = make(map[uint64]*cachedVolume)
	}
	entry := c.devices[key]
	if entry == nil {
		entry = &cachedVolume{}
		c.devices[key] = entry
	}
	c.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !time.Now().Before(entry.expires) {
		if mount, ok := lookup(key); ok {
			value := fromMountPoint(mount.path)
			if mount.queryUUID {
				uuid, answer := query(mount.path)
				switch answer {
				case uuidFound:
					value = fromUUID(uuid)
				case uuidFailed:
					if entry.mount == mount && entry.value != 0 {
						value = entry.value
					}
				case uuidNone:
				}
			}
			entry.mount, entry.value = mount, value
		}
		entry.expires = time.Now().Add(refreshAfter)
	}
	if entry.value != 0 {
		return entry.value
	}
	return dev
}

func fromUUID(uuid [16]byte) uint64 { return hashValue("uuid:", uuid[:]) }

func fromMountPoint(mount string) uint64 { return hashValue("path:", []byte(mount)) }

// Values must fit the archive's signed integer and cannot use its zero sentinel.
func hashValue(tag string, id []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(tag))
	_, _ = h.Write(id)
	if v := h.Sum64() &^ (1 << 63); v != 0 {
		return v
	}
	return 1
}
