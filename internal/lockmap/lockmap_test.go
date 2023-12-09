package lockmap_test

import (
	"testing"

	"github.com/michaelpeterswa/yt-playlist-ripper/internal/lockmap"
	"github.com/stretchr/testify/assert"
)

func TestLockMapAdd(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "add one key",
			keys: []string{"key1"},
		},
		{
			name: "add three keys",
			keys: []string{"key1", "key2", "key3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				if err := lm.Add(key); err != nil {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestFailLockMapAdd(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "add one key twice",
			keys: []string{"key1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				if err := lm.Add(key); err != nil {
					assert.NoError(t, err)
				}
				if err := lm.Add(key); err == nil {
					assert.ErrorIs(t, err, lockmap.ErrorLockMapKeyAlreadyExists)
				}
			}
		})
	}
}

func TestLockMapLockKey(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "lock one key",
			keys: []string{"key1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				_ = lm.Add(key)
			}

			for _, key := range tc.keys {
				if err := lm.Lock(key); err != nil {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestFailLockMapLockKey(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "fail lock missing key",
			keys: []string{"key1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				if err := lm.Lock(key); err == nil {
					assert.ErrorIs(t, err, lockmap.ErrorLockMapKeyNotFound)
				}
			}
		})
	}
}

func TestLockMapUnlockKey(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "unlock one key",
			keys: []string{"key1"},
		},
		{
			name: "unlock three keys",
			keys: []string{"key1", "key2", "key3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				_ = lm.Add(key)
			}

			for _, key := range tc.keys {
				if err := lm.Lock(key); err != nil {
					assert.NoError(t, err)
				}

				if err := lm.Unlock(key); err != nil {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestFailLockMapUnlockKey(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "fail unlock missing key",
			keys: []string{"key1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lm := lockmap.New()

			for _, key := range tc.keys {
				if err := lm.Unlock(key); err == nil {
					assert.ErrorIs(t, err, lockmap.ErrorLockMapKeyNotFound)
				}
			}
		})
	}
}
