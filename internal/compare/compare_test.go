package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.bin")
	content := []byte("case evidence\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	got, size, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if got != hex.EncodeToString(sum[:]) || size != int64(len(content)) {
		t.Fatalf("got %s size %d", got, size)
	}
}

func TestFind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	rec, err := chain.New(path)
	if err != nil {
		t.Fatal(err)
	}
	itemBytes := sha256.Sum256([]byte("item"))
	itemHash := hex.EncodeToString(itemBytes[:])
	metaBytes := sha256.Sum256([]byte("metadata"))
	metaHash := hex.EncodeToString(metaBytes[:])

	item := model.Item{
		ID: "E-0001", Name: "capture.html", StoredPath: "items/E-0001__capture.html",
		SHA256: itemHash, Metadata: model.MetadataRef{SHA256: metaHash},
	}
	itemRec, err := rec.Add("item_acquired", item.ID, "fs", item)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Add("item_metadata", item.ID, "stratum", model.ItemMetadataData{
		ItemID: item.ID, Metadata: model.MetadataRef{SHA256: metaHash},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := chain.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Report.OK {
		t.Fatalf("chain: %+v", loaded.Report)
	}

	matches, err := Find(loaded.Records, itemHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("item matches = %+v", matches)
	}
	if matches[0].Field != "data.sha256" || matches[0].ItemID != "E-0001" || matches[0].ItemName != "capture.html" {
		t.Fatalf("item match = %+v", matches[0])
	}
	if matches[0].StoredPath != item.StoredPath {
		t.Fatalf("stored path = %q", matches[0].StoredPath)
	}

	matches, err = Find(loaded.Records, metaHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("metadata matches = %+v", matches)
	}
	for _, m := range matches {
		if m.ItemID != "E-0001" {
			t.Fatalf("metadata item = %+v", m)
		}
	}

	matches, err = Find(loaded.Records, itemRec.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ItemID != "E-0001" || matches[0].Field != "hash" {
		t.Fatalf("record match = %+v", matches)
	}

	matches, err = Find(loaded.Records, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected matches = %+v", matches)
	}

	if _, err := Find(loaded.Records, "nope"); err == nil {
		t.Fatal("expected invalid digest error")
	}
}
