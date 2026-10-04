package rawcapture

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturerPersistsVolumeIdentityAndChecksLiveFileIdentity(t *testing.T) {
	store, _ := openCapturerTestStore(t, 1<<20)
	provider, source, path := captureFileProvider(t, "one\n")
	capturer := New(store)
	first, err := capturer.Capture(t.Context(), provider, source)
	require.NoError(t, err)
	base, ok, err := store.CaptureBase(t.Context(), first.Source)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, base.Entries, 1)
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	info, err := file.Stat()
	require.NoError(t, err)
	assert.NotEqual(t, stableFileIdentity(file, info), base.Entries[0].FileIdentity)

	require.NoError(t, appendFile(path, "two\n"))
	second, err := capturer.Capture(t.Context(), provider, source)
	require.NoError(t, err)
	updated, ok, err := store.CaptureBase(t.Context(), second.Source)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, updated.Entries, 1)
	assert.Equal(t, base.Entries[0].FileIdentity, updated.Entries[0].FileIdentity)
	assert.Equal(t, int64(8), updated.Entries[0].Length)
	assert.Len(t, updated.Entries[0].Objects, 2)
}
