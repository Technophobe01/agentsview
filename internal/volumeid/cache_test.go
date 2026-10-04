package volumeid

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheQueriesOnlyRequestedVolume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache volumeCache
		mounts := map[uint64]mountedVolume{
			1: {path: "/data", from: "/dev/disk1", queryUUID: true},
			2: {path: "/unused", from: "/dev/disk2", queryUUID: true},
		}
		lookups, queries := 0, 0
		lookup := func(dev uint64) (mountedVolume, bool) {
			lookups++
			mount, ok := mounts[dev]
			return mount, ok
		}
		query := func(path string) ([16]byte, uuidAnswer) {
			queries++
			assert.Equal(t, "/data", path)
			return [16]byte{1}, uuidFound
		}
		first := cache.stable(1, lookup, query)
		for range 10 {
			assert.Equal(t, first, cache.stable(1, lookup, query))
		}
		assert.Equal(t, 1, lookups)
		assert.Equal(t, 1, queries)
		time.Sleep(time.Minute)
		assert.Equal(t, 1, queries, "idle volumes do no work")
		assert.Equal(t, first, cache.stable(1, lookup, query))
		assert.Equal(t, 2, lookups)
		assert.Equal(t, 2, queries)
	})
}

func TestCacheFollowsVolumeAcrossReboots(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		uuid [16]byte
		same bool
	}{
		{"same volume", "/data", [16]byte{1}, true},
		{"different mount path", "/relocated", [16]byte{1}, true},
		{"replacement volume", "/data", [16]byte{2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var before, after volumeCache
			mount := mountedVolume{path: "/data", queryUUID: true}
			lookup := func(uint64) (mountedVolume, bool) { return mount, true }
			uuid := [16]byte{1}
			query := func(string) ([16]byte, uuidAnswer) { return uuid, uuidFound }
			first := before.stable(1, lookup, query)
			mount.path, uuid = tc.path, tc.uuid
			second := after.stable(2, lookup, query)
			assert.NotZero(t, first)
			assert.Less(t, first, uint64(1)<<63)
			if tc.same {
				assert.Equal(t, first, second)
			} else {
				assert.NotEqual(t, first, second)
			}
		})
	}
}

func TestCacheRefreshesAReplacementAtTheSameMount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache volumeCache
		lookup := func(uint64) (mountedVolume, bool) {
			return mountedVolume{path: "/data", from: "/dev/disk1", queryUUID: true}, true
		}
		uuid := [16]byte{1}
		query := func(string) ([16]byte, uuidAnswer) { return uuid, uuidFound }
		first := cache.stable(1, lookup, query)
		uuid = [16]byte{2}
		assert.Equal(t, first, cache.stable(1, lookup, query), "cached until expiry")
		time.Sleep(time.Minute)
		assert.NotEqual(t, first, cache.stable(1, lookup, query))
	})
}

func TestCacheKeepsKnownIdentityOnFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache volumeCache
		mount := mountedVolume{path: "/data", from: "/dev/disk1", queryUUID: true}
		mountOK := true
		lookup := func(uint64) (mountedVolume, bool) { return mount, mountOK }
		answer := uuidFound
		query := func(string) ([16]byte, uuidAnswer) { return [16]byte{1}, answer }
		first := cache.stable(1, lookup, query)
		answer = uuidFailed
		time.Sleep(time.Minute)
		assert.Equal(t, first, cache.stable(1, lookup, query))
		mountOK = false
		time.Sleep(time.Minute)
		assert.Equal(t, first, cache.stable(1, lookup, query))
		mountOK, mount.from = true, "/dev/disk2"
		time.Sleep(time.Minute)
		assert.NotEqual(t, first, cache.stable(1, lookup, query), "a different mount cannot inherit the old UUID")
	})
}

func TestCacheUsesMountPathWithoutUUID(t *testing.T) {
	for _, queryUUID := range []bool{false, true} {
		var before, after volumeCache
		mount := mountedVolume{path: "/share", queryUUID: queryUUID}
		lookup := func(uint64) (mountedVolume, bool) { return mount, true }
		queries := 0
		query := func(string) ([16]byte, uuidAnswer) {
			queries++
			return [16]byte{}, uuidNone
		}
		first := before.stable(1, lookup, query)
		assert.NotEqual(t, uint64(1), first)
		assert.Equal(t, first, after.stable(2, lookup, query))
		if !queryUUID {
			assert.Zero(t, queries, "network and automount volumes are not queried")
		}
	}
}

func TestCacheRetriesAnInitiallyUnknownVolume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache volumeCache
		found := false
		lookup := func(uint64) (mountedVolume, bool) {
			return mountedVolume{path: "/data", queryUUID: true}, found
		}
		answer := uuidFailed
		query := func(string) ([16]byte, uuidAnswer) { return [16]byte{1}, answer }
		var dev int32 = -1
		assert.Equal(t, uint64(dev), cache.stable(uint64(dev), lookup, query))
		found = true
		time.Sleep(time.Minute)
		fallback := cache.stable(uint64(dev), lookup, query)
		assert.NotEqual(t, uint64(dev), fallback)
		answer = uuidFound
		time.Sleep(time.Minute)
		stable := cache.stable(uint64(dev), lookup, query)
		assert.NotEqual(t, fallback, stable)
		assert.Equal(t, stable, cache.stable(uint64(uint32(dev)), lookup, query))
	})
}

func TestCacheSlowVolumeDoesNotBlockAnotherDevice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var cache volumeCache
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan uint64, 1)
		lookup := func(dev uint64) (mountedVolume, bool) {
			if dev == 1 {
				return mountedVolume{path: "/slow", queryUUID: true}, true
			}
			return mountedVolume{path: "/fast", queryUUID: true}, true
		}
		query := func(path string) ([16]byte, uuidAnswer) {
			if path == "/slow" {
				close(entered)
				<-release
				return [16]byte{1}, uuidFound
			}
			return [16]byte{2}, uuidFound
		}
		go func() { done <- cache.stable(1, lookup, query) }()
		<-entered
		fast := cache.stable(2, lookup, query)
		assert.NotEqual(t, uint64(2), fast)
		close(release)
		assert.NotEqual(t, fast, <-done)
	})
}

func TestCacheConcurrentLookupsShareOneQuery(t *testing.T) {
	var cache volumeCache
	lookup := func(dev uint64) (mountedVolume, bool) {
		assert.Equal(t, uint64(1), dev)
		return mountedVolume{path: "/data", queryUUID: true}, true
	}
	queries := 0
	query := func(string) ([16]byte, uuidAnswer) {
		queries++
		return [16]byte{1}, uuidFound
	}
	var wg sync.WaitGroup
	values := make([]uint64, 16)
	for i := range values {
		wg.Go(func() { values[i] = cache.stable(1, lookup, query) })
	}
	wg.Wait()
	require.Equal(t, 1, queries)
	for _, value := range values {
		assert.NotZero(t, value)
		assert.Equal(t, values[0], value)
	}
}
