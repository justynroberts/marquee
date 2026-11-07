package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

// Key is a stored API key. Only a SHA-256 hash of the secret is kept on disk.
type Key struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Prefix  string    `json:"prefix"` // first characters, to recognise a key in lists
	Hash    string    `json:"hash"`
	Created time.Time `json:"created"`
	LastUse time.Time `json:"lastUsed,omitzero"`
}

// Keys is the on-disk key store (keys.json, mode 0600).
type Keys struct {
	path string
	mu   sync.Mutex
	list []Key
}

// LoadKeys opens (or creates) the key store.
func LoadKeys(path string) (*Keys, error) {
	k := &Keys{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &k.list); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return k, nil
}

func (k *Keys) save() error {
	data, _ := json.MarshalIndent(k.list, "", "  ")
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

// Create makes a new key and returns its secret (shown once).
func (k *Keys) Create(name string) (Key, string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Key{}, "", err
	}
	secret := "mq_" + hex.EncodeToString(raw)
	id := make([]byte, 4)
	_, _ = rand.Read(id)
	sum := sha256.Sum256([]byte(secret))
	key := Key{ID: hex.EncodeToString(id), Name: name, Prefix: secret[:9], Hash: hex.EncodeToString(sum[:]), Created: time.Now().UTC()}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.list = append(k.list, key)
	return key, secret, k.save()
}

// List returns the keys (without hashes).
func (k *Keys) List() []Key {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]Key, len(k.list))
	for i, key := range k.list {
		key.Hash = ""
		out[i] = key
	}
	return out
}

// Revoke deletes a key by id.
func (k *Keys) Revoke(id string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	for i, key := range k.list {
		if key.ID == id {
			k.list = append(k.list[:i], k.list[i+1:]...)
			_ = k.save()
			return true
		}
	}
	return false
}

// Check reports whether secret matches a stored key (constant-time per key).
func (k *Keys) Check(secret string) bool {
	if secret == "" {
		return false
	}
	sum := sha256.Sum256([]byte(secret))
	h := hex.EncodeToString(sum[:])
	k.mu.Lock()
	defer k.mu.Unlock()
	ok := false
	for i := range k.list {
		if subtle.ConstantTimeCompare([]byte(h), []byte(k.list[i].Hash)) == 1 {
			k.list[i].LastUse = time.Now().UTC()
			ok = true
		}
	}
	return ok
}
