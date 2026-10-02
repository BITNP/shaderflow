package archive

import (
	"shaderflow/internal/game"
	"testing"
)

func TestReplayFromNearestCheckpoint(t *testing.T) {
	start := game.State{Phase: "running", Codes: map[string]game.Code{}}
	checkpoint := Checkpoint{Seq: 10, State: game.State{Phase: "running", Seq: 10, Codes: map[string]game.Code{"n0": {Version: 1, Code: "old"}}}}
	events := []game.Envelope{{Seq: 11, Event: &game.Event{Type: "code.updated", Data: map[string]any{"nodeId": "n0", "code": map[string]any{"version": float64(2), "code": "new"}}}}}
	state, err := ReplayFrom(start, events, []Checkpoint{checkpoint}, 11)
	if err != nil {
		t.Fatal(err)
	}
	if state.Codes["n0"].Version != 2 || state.Seq != 11 {
		t.Fatalf("wrong state: %#v", state)
	}
}

func TestReplayReducerAppliesAcceptedCodeAndTrajectoryEvents(t *testing.T) {
	start := game.State{Phase: "running", Settings: game.DefaultSettings(), Codes: map[string]game.Code{}}
	events := []game.Envelope{
		{Seq: 1, Event: &game.Event{Type: "code.updated", Data: map[string]any{"nodeId": "n0", "code": map[string]any{"version": float64(7), "code": "// v7", "sub": "u"}}}},
		{Seq: 2, Event: &game.Event{Type: "trajectory.updated", Data: map[string]any{"version": float64(3), "name": "Orbit", "frameSource": "fov=60;", "validatorVersion": "camera-v1", "active": true}}},
	}
	state, err := Replay(start, events, 2)
	if err != nil {
		t.Fatal(err)
	}
	if state.Codes["n0"].Version != 7 {
		t.Fatalf("code not replayed: %#v", state.Codes)
	}
	if state.Trajectory == nil || state.Trajectory.Version != 3 {
		t.Fatalf("trajectory not replayed: %#v", state.Trajectory)
	}
	before, err := Replay(start, events, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Trajectory != nil {
		t.Fatal("replay crossed requested event sequence")
	}
}
