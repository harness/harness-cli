package credential

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_SetGetDelete(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{path: filepath.Join(dir, "creds.json")}

	err := store.Set("acct-1", "token-abc")
	require.NoError(t, err)

	tok, err := store.Get("acct-1")
	require.NoError(t, err)
	assert.Equal(t, "token-abc", tok)

	err = store.Delete("acct-1")
	require.NoError(t, err)

	_, err = store.Get("acct-1")
	assert.Error(t, err)
}

func TestFileStore_GetMissing(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{path: filepath.Join(dir, "creds.json")}

	_, err := store.Get("nonexistent")
	assert.Error(t, err)
}

func TestFileStore_MultipleAccounts(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{path: filepath.Join(dir, "creds.json")}

	require.NoError(t, store.Set("acct-1", "tok-1"))
	require.NoError(t, store.Set("acct-2", "tok-2"))

	tok1, err := store.Get("acct-1")
	require.NoError(t, err)
	assert.Equal(t, "tok-1", tok1)

	tok2, err := store.Get("acct-2")
	require.NoError(t, err)
	assert.Equal(t, "tok-2", tok2)

	require.NoError(t, store.Delete("acct-1"))

	_, err = store.Get("acct-1")
	assert.Error(t, err)

	tok2, err = store.Get("acct-2")
	require.NoError(t, err)
	assert.Equal(t, "tok-2", tok2)
}

func TestFileStore_Permissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	store := &FileStore{path: path}

	require.NoError(t, store.Set("acct-1", "secret"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestNewStore_Insecure(t *testing.T) {
	store := NewStore(true)
	_, ok := store.(*FileStore)
	assert.True(t, ok)
}

func TestNewStore_Secure(t *testing.T) {
	store := NewStore(false)
	_, ok := store.(*KeychainStore)
	assert.True(t, ok)
}
