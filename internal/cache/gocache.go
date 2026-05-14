package cache

import (
	"path/filepath"
	"time"

	"github.com/lyonbrown4d/bu1ld/internal/cachefile"
	"github.com/lyonbrown4d/bu1ld/internal/snapshot"

	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type GoCacheEntry struct {
	ActionID string    `json:"actionId,omitempty"`
	OutputID string    `json:"outputId"`
	Size     int64     `json:"size"`
	Time     time.Time `json:"time"`
}

func (s *Store) LoadGoCacheEntry(actionID string) (GoCacheEntry, bool, error) {
	if err := validateStoreCacheKey(actionID, "go cache action id"); err != nil {
		return GoCacheEntry{}, false, err
	}

	path := s.goCacheActionPath(actionID)
	if _, err := s.fs.Stat(path); err != nil {
		if isNotExist(err) {
			return GoCacheEntry{}, false, nil
		}
		return GoCacheEntry{}, false, oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("path", path).
			Wrapf(err, "stat go cache action")
	}

	var entry GoCacheEntry
	if err := cachefile.Read(s.fs, path, &entry); err != nil {
		return GoCacheEntry{}, false, oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("path", path).
			Wrapf(err, "read go cache action")
	}
	if entry.ActionID != "" && entry.ActionID != actionID {
		return GoCacheEntry{}, false, oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("record_action_id", entry.ActionID).
			New("go cache action id mismatch")
	}
	if err := validateStoreCacheKey(entry.OutputID, "go cache output id"); err != nil {
		return GoCacheEntry{}, false, err
	}
	entry.ActionID = actionID
	return entry, true, nil
}

func (s *Store) SaveGoCacheEntry(actionID string, entry GoCacheEntry) error {
	if err := validateStoreCacheKey(actionID, "go cache action id"); err != nil {
		return err
	}
	if entry.ActionID != "" && entry.ActionID != actionID {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("record_action_id", entry.ActionID).
			New("go cache action id mismatch")
	}
	if err := validateStoreCacheKey(entry.OutputID, "go cache output id"); err != nil {
		return err
	}
	if entry.Size < 0 {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("size", entry.Size).
			New("go cache output size must be non-negative")
	}
	info, err := s.fs.Stat(s.blobPath(entry.OutputID))
	if err != nil {
		if isNotExist(err) {
			return oops.In("bu1ld.cache.go").
				With("action_id", actionID).
				With("output_id", entry.OutputID).
				New("go cache action references missing output")
		}
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("output_id", entry.OutputID).
			Wrapf(err, "stat go cache output")
	}
	if entry.Size != info.Size() {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			With("output_id", entry.OutputID).
			With("record_size", entry.Size).
			With("output_size", info.Size()).
			New("go cache output size mismatch")
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	entry.ActionID = actionID
	data, err := cachefile.Marshal(entry)
	if err != nil {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			Wrapf(err, "encode go cache action")
	}
	if err := s.checkObjectSize(int64(len(data)), "go cache action"); err != nil {
		return err
	}
	if err := atomicWriteFile(s.fs, s.goCacheActionPath(actionID), data, 0o644); err != nil {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			Wrapf(err, "write go cache action")
	}
	if err := s.EnforcePolicy(); err != nil {
		return oops.In("bu1ld.cache.go").
			With("action_id", actionID).
			Wrapf(err, "enforce cache policy")
	}
	return nil
}

func (s *Store) LoadGoCacheOutput(outputID string) ([]byte, bool, error) {
	if err := validateStoreCacheKey(outputID, "go cache output id"); err != nil {
		return nil, false, err
	}
	data, err := afero.ReadFile(s.fs, s.blobPath(outputID))
	if err != nil {
		if isNotExist(err) {
			return nil, false, nil
		}
		return nil, false, oops.In("bu1ld.cache.go").
			With("output_id", outputID).
			Wrapf(err, "read go cache output")
	}
	if snapshot.HashBytes(data) != outputID {
		return nil, false, oops.In("bu1ld.cache.go").
			With("output_id", outputID).
			New("go cache output digest mismatch")
	}
	return data, true, nil
}

func (s *Store) SaveGoCacheOutput(outputID string, data []byte) error {
	if err := validateStoreCacheKey(outputID, "go cache output id"); err != nil {
		return err
	}
	if snapshot.HashBytes(data) != outputID {
		return oops.In("bu1ld.cache.go").
			With("output_id", outputID).
			New("go cache output digest mismatch")
	}
	return s.writeBlobBytes(outputID, data)
}

func (s *Store) goCacheActionPath(actionID string) string {
	prefix := "00"
	if len(actionID) >= 2 {
		prefix = actionID[:2]
	}
	return filepath.Join(s.cfg.CachePath(), "go", "actions", prefix, actionID+".bin")
}

func validateStoreCacheKey(value, label string) error {
	if isCacheKey(value) {
		return nil
	}
	return oops.In("bu1ld.cache").
		With("value", value).
		Errorf("invalid %s", label)
}

func isCacheKey(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return false
	}
	return true
}
