package archive

import (
	"os"
	"path/filepath"
	"shaderflow/internal/game"
	"testing"
)

func TestDiagnoseAndRepairTruncatedEventTail(t *testing.T) {
	root := t.TempDir()
	store := NewFilesystem(root)
	if err := store.Start(game.State{Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(game.Envelope{Seq: 1, Event: &game.Event{Type: "time.controlled", Data: game.TimeControl{Version: 1, Action: "pause"}}}); err != nil {
		t.Fatal(err)
	}
	id := store.manifest.RoundID
	events := filepath.Join(root, id, "events.jsonl")
	file, err := os.OpenFile(events, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"seq":2`); err != nil {
		t.Fatal(err)
	}
	file.Close()
	report, err := NewFilesystem(root).Diagnose(id)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || report.LastValidSeq != 1 || !report.TruncatedTail {
		t.Fatalf("bad diagnosis: %#v", report)
	}
	repaired, err := NewFilesystem(root).Repair(id, RepairOptions{TruncateInvalidTail: true, CloseRound: true})
	if err != nil {
		t.Fatal(err)
	}
	if !repaired.Healthy {
		t.Fatalf("repair unhealthy: %#v", repaired)
	}
	round, err := NewFilesystem(root).Read(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(round.Events) != 1 || round.Manifest.Status != "complete" {
		t.Fatalf("bad repaired round: %#v", round)
	}
}
