package credential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "harness-cli"
	tokenKey    = "api-token"
)

type Store interface {
	Set(accountID, token string) error
	Get(accountID string) (string, error)
	Delete(accountID string) error
}

type KeychainStore struct{}

func (k *KeychainStore) Set(accountID, token string) error {
	return keyring.Set(serviceName, accountID, token)
}

func (k *KeychainStore) Get(accountID string) (string, error) {
	return keyring.Get(serviceName, accountID)
}

func (k *KeychainStore) Delete(accountID string) error {
	return keyring.Delete(serviceName, accountID)
}

type FileStore struct {
	path string
}

func NewFileStore() (*FileStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	return &FileStore{path: filepath.Join(homeDir, ".harness", "credentials.json")}, nil
}

func (f *FileStore) Set(accountID, token string) error {
	data := f.readAll()
	if data == nil {
		data = make(map[string]string)
	}
	data[accountID] = token
	return f.writeAll(data)
}

func (f *FileStore) Get(accountID string) (string, error) {
	data := f.readAll()
	if data == nil {
		return "", fmt.Errorf("no credentials found")
	}
	tok, ok := data[accountID]
	if !ok {
		return "", fmt.Errorf("no token found for account %s", accountID)
	}
	return tok, nil
}

func (f *FileStore) Delete(accountID string) error {
	data := f.readAll()
	if data == nil {
		return nil
	}
	delete(data, accountID)
	if len(data) == 0 {
		return os.Remove(f.path)
	}
	return f.writeAll(data)
}

func (f *FileStore) readAll() map[string]string {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return nil
	}
	var data map[string]string
	if json.Unmarshal(raw, &data) != nil {
		return nil
	}
	return data
}

func (f *FileStore) writeAll(data map[string]string) error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, raw, 0600)
}

func NewStore(insecure bool) (Store, error) {
	if insecure {
		return NewFileStore()
	}
	return &KeychainStore{}, nil
}

func IsKeyringAvailable() bool {
	err := keyring.Set(serviceName, "__probe__", "test")
	if err != nil {
		return false
	}
	defer keyring.Delete(serviceName, "__probe__") //nolint:errcheck
	return true
}
