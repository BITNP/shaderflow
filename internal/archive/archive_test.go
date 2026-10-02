package archive

import (
	"errors"
	"os"
	"path/filepath"
	"shaderflow/internal/game"
	"strings"
	"testing"
	"time"
)

func TestGarbageCollectBlobsDryRunAgeAndReferences(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	if err := store.Start(game.State{Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	ref, err := store.StoreBlob("image/png", []byte("referenced"))
	if err != nil {
		t.Fatal(err)
	}
	input := game.ExternalInput{ID: "i", RoundID: ref.RoundID, Version: 1, SHA256: ref.SHA256, Size: ref.Size, ContentType: ref.ContentType}
	if err := store.Append(game.Envelope{Seq: 1, Event: &game.Event{Type: "input.updated", Data: input}}); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, ref.RoundID, "blobs", strings.Repeat("a", 64))
	if err := os.WriteFile(orphan, []byte("orphan"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := store.GarbageCollectBlobs(GCOptions{MinimumAge: time.Hour, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.OrphanCount != 1 || report.DeletedCount != 0 {
		t.Fatalf("bad dry run: %#v", report)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("dry run deleted orphan")
	}
	report, err = store.GarbageCollectBlobs(GCOptions{MinimumAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if report.DeletedCount != 1 {
		t.Fatalf("orphan not deleted: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, ref.RoundID, "blobs", ref.SHA256)); err != nil {
		t.Fatal("referenced blob deleted")
	}
}

func TestCatalogIncludesActiveRoundAndBlobRollback(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	if err := store.Start(game.State{Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	active, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Status != "active" {
		t.Fatalf("active round absent: %#v", active)
	}
	ref, err := store.StoreBlob("image/png", []byte("blob"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBlob(ref); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].BlobCount != 0 || items[0].BlobBytes != 0 {
		t.Fatalf("rolled back blob counted: %#v", items[0])
	}
}

func TestCatalogPersistsRoundStatisticsAndRebuildsWhenMissing(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	if err := store.Start(game.State{Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StoreBlob("image/png", []byte("blob")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(game.Envelope{Seq: 1, Event: &game.Event{Type: "time.controlled", Data: game.TimeControl{Version: 1, Action: "pause"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	items, err := NewFilesystem(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EventCount != 1 || items[0].BlobCount != 1 || items[0].BlobBytes != 4 {
		t.Fatalf("bad catalog: %#v", items)
	}
	if err := os.Remove(filepath.Join(root, "catalog.json")); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := NewFilesystem(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt) != 1 || rebuilt[0].RoundID != items[0].RoundID {
		t.Fatalf("catalog not rebuilt: %#v", rebuilt)
	}
}

func TestFilesystemCreatesListsAndReadsVersionedRound(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	state := game.State{Phase: "running", Settings: game.DefaultSettings(), Seq: 4}
	if err := store.Start(state); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(game.Envelope{Type: "event", Seq: 5, Event: &game.Event{Type: "trajectory.updated", Data: game.Trajectory{Version: 1, Active: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SchemaVersion != 1 || list[0].Status != "complete" {
		t.Fatalf("unexpected list: %#v", list)
	}
	round, err := store.Read(list[0].RoundID)
	if err != nil {
		t.Fatal(err)
	}
	if round.Start.Seq != 4 || len(round.Events) != 1 || round.Events[0].Seq != 5 {
		t.Fatalf("unexpected round: %#v", round)
	}
}

func TestFilesystemRemembersRuntimeFailureAndBlocksFurtherWrites(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	store.failWrites = errors.New("disk full")
	if err := store.Start(game.State{Phase: "running"}); err == nil {
		t.Fatal("start failure not exposed")
	}
	if store.Status().Ready || store.Status().Error == "" {
		t.Fatalf("failure status missing: %#v", store.Status())
	}
	store.failWrites = nil
	if err := store.Start(game.State{Phase: "running"}); err == nil {
		t.Fatal("degraded archive accepted later write")
	}
}

func TestFilesystemPersistsPeriodicCheckpoints(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	state := game.State{Phase: "running", Settings: game.DefaultSettings()}
	if err := store.Start(state); err != nil {
		t.Fatal(err)
	}
	for seq := uint64(1); seq <= CheckpointInterval; seq++ {
		if err := store.Append(game.Envelope{Seq: seq, Event: &game.Event{Type: "time.controlled", Data: game.TimeControl{Version: int64(seq), Action: "pause"}}}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	round, err := store.Read(list[0].RoundID)
	if err != nil {
		t.Fatal(err)
	}
	if len(round.Checkpoints) != 1 || round.Checkpoints[0].Seq != CheckpointInterval {
		t.Fatalf("bad checkpoints: %#v", round.Checkpoints)
	}
}

func TestFilesystemStoresAndVerifiesContentAddressedBlob(t *testing.T) {
	store := NewFilesystem(t.TempDir())
	if err := store.Start(game.State{Phase: "running", Settings: game.DefaultSettings()}); err != nil {
		t.Fatal(err)
	}
	ref, err := store.StoreBlob("image/png", []byte("png bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if ref.SHA256 == "" || ref.Size != 9 {
		t.Fatalf("bad ref: %#v", ref)
	}
	data, contentType, err := store.ReadBlob(ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png bytes" || contentType != "image/png" {
		t.Fatalf("bad blob %q %q", data, contentType)
	}
	ref.SHA256 = "00"
	if _, _, err := store.ReadBlob(ref); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestFilesystemDetectsLegacyAndCorruptArchives(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "20240101-120000.000000000")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "start.json"), []byte(`{"phase":"running"}`), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(root, "broken")
	if err := os.MkdirAll(corrupt, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corrupt, "manifest.json"), []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := NewFilesystem(root).List()
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	for _, item := range list {
		status[item.RoundID] = item.Status
	}
	if status[filepath.Base(legacy)] != "legacy" {
		t.Fatalf("legacy not detected: %#v", status)
	}
	if status[filepath.Base(corrupt)] != "corrupt" {
		t.Fatalf("corrupt not detected: %#v", status)
	}
}
