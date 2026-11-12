package groundtruth

import (
	"testing"
	"time"
)

func TestCommitKeepsScheduleSealedUntilUnseal(t *testing.T) {
	log := New([]Entry{{ScenarioID: 1, TrueLayers: Layers{Model: true}}})
	commitment, err := log.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(Entry{ScenarioID: 1, TimestampUTC: time.Now(), TrueLayers: Layers{Model: true}}); err != nil {
		t.Fatal(err)
	}
	entries, err := log.Unseal(commitment)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].TrueLayers.Model {
		t.Fatalf("entries = %+v", entries)
	}
	if _, err := log.Unseal(commitment); err == nil {
		t.Fatal("second unseal unexpectedly succeeded")
	}
}
