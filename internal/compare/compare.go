// Package compare checks a SHA-256 digest against hashes recorded in a case
// evidence ledger.
package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

// Match identifies one place in the evidence where a digest appears.
type Match struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"`
	Target     string `json:"target"`
	Source     string `json:"source"`
	Field      string `json:"field"`
	ItemID     string `json:"item_id,omitempty"`
	ItemName   string `json:"item_name,omitempty"`
	StoredPath string `json:"stored_path,omitempty"`
}

// FileSHA256 streams path through SHA-256 and returns its digest and size.
func FileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if !st.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s is not a regular file", path)
	}

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// NormalizeDigest accepts a SHA-256 with an optional "sha256:" prefix and
// returns its lowercase hex form.
func NormalizeDigest(s string) (string, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "sha256:")
	if len(s) != 64 {
		return "", fmt.Errorf("invalid SHA-256 digest: expected 64 hex characters")
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("invalid SHA-256 digest: %v", err)
	}
	return s, nil
}

// Find searches validated records for digest. The caller should verify the
// chain first; app.compare does this before calling Find.
func Find(records []chain.Record, digest string) ([]Match, error) {
	target, err := NormalizeDigest(digest)
	if err != nil {
		return nil, err
	}

	items := make(map[string]model.Item)
	for _, rec := range records {
		if rec.Type != "item_acquired" {
			continue
		}
		var item model.Item
		if json.Unmarshal(rec.Data, &item) == nil && item.ID != "" {
			items[item.ID] = item
		}
	}

	var matches []Match
	for _, rec := range records {
		itemID := ""
		if rec.Type == "item_metadata" {
			var md model.ItemMetadataData
			if json.Unmarshal(rec.Data, &md) == nil {
				itemID = md.ItemID
			}
		}
		if itemID == "" {
			if _, ok := items[rec.Target]; ok {
				itemID = rec.Target
			}
		}
		item := items[itemID]

		add := func(field, value string) {
			if value == "" || !strings.EqualFold(value, target) {
				return
			}
			matches = append(matches, Match{
				Seq:        rec.Seq,
				Type:       rec.Type,
				Target:     rec.Target,
				Source:     rec.Source,
				Field:      field,
				ItemID:     itemID,
				ItemName:   item.Name,
				StoredPath: item.StoredPath,
			})
		}

		add("hash", rec.Hash)
		if len(rec.Data) > 0 {
			var data any
			if err := json.Unmarshal(rec.Data, &data); err != nil {
				return nil, fmt.Errorf("parse evidence data at seq %d: %w", rec.Seq, err)
			}
			walkHashFields(data, "data", add)
		}
	}
	return matches, nil
}

func walkHashFields(v any, path string, add func(field, value string)) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if path != "" {
				next = path + "." + key
			}
			walkHashFields(x[key], next, add)
		}
	case []any:
		for i, item := range x {
			walkHashFields(item, fmt.Sprintf("%s[%d]", path, i), add)
		}
	case string:
		if isSHA256(x) {
			add(path, x)
		}
	}
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
