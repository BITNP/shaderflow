package game

import (
	"errors"
	"testing"
)

type lifecycleRecorder struct {
	starts    int
	closes    int
	appendErr error
}

func (r *lifecycleRecorder) Start(State) error       { r.starts++; return nil }
func (r *lifecycleRecorder) Append(Envelope) error   { return r.appendErr }
func (r *lifecycleRecorder) Code(string, Code) error { return nil }
func (r *lifecycleRecorder) Close() error            { r.closes++; return nil }

func TestGeneratedGraphRandomizesInputArityWithinConfiguredRange(t *testing.T) {
	s := DefaultSettings()
	s.Players = 5
	s.InputCount = 1
	s.MinInputs = 1
	s.MaxInputs = 3
	s.MinOutputs = 1
	s.MaxOutputs = 1
	seen := map[int]bool{}
	for attempt := 0; attempt < 40; attempt++ {
		graph, err := Generate(s)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range graph.Nodes {
			if len(node.Inputs) < 1 || len(node.Inputs) > 3 {
				t.Fatalf("input arity out of range: %d", len(node.Inputs))
			}
			seen[len(node.Inputs)] = true
		}
	}
	if len(seen) < 2 {
		t.Fatalf("input arity was not randomized: %#v", seen)
	}
}

func TestGeneratedGraphValid(t *testing.T) {
	s := DefaultSettings()
	s.Players = 5
	s.InputCount = 3
	s.MinInputs = 1
	s.MaxInputs = 2
	s.MinOutputs = 1
	s.MaxOutputs = 2
	for i := 0; i < 20; i++ {
		g, err := Generate(s)
		if err != nil {
			t.Fatalf("generate %d: %v", i, err)
		}
		if err := g.Validate(s.Players); err != nil {
			t.Fatal(err)
		}
	}
}
func TestGraphRejectsCycleAndDeadTerminal(t *testing.T) {
	g := Graph{Inputs: []string{"input0"}, Nodes: []Node{{ID: "n0", Inputs: []string{"n1:0"}, Outputs: []string{"n0:0"}}, {ID: "n1", Inputs: []string{"n0:0"}, Outputs: []string{"n1:0"}}}, Output: "n1:0"}
	if err := g.Validate(2); err == nil {
		t.Fatal("cycle accepted")
	}
	g = Graph{Inputs: []string{"input0"}, Nodes: []Node{{ID: "n0", Inputs: []string{"input0"}, Outputs: []string{"n0:0", "n0:1"}}}, Output: "n0:0"}
	if err := g.Validate(1); err == nil {
		t.Fatal("dead output accepted")
	}
}
func TestPlayerSnapshotRestoresOnlyOwnAcceptedShaderVersion(t *testing.T) {
	g := New(nil)
	s := DefaultSettings()
	s.Players = 2
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("alice", "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("bob", "Bob"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	if err := g.Upload("alice", 7, "// alice accepted"); err != nil {
		t.Fatal(err)
	}
	if err := g.Upload("bob", 9, "// bob accepted"); err != nil {
		t.Fatal(err)
	}

	alice := g.Snapshot("alice", false)
	if alice.OwnCode == nil {
		t.Fatal("player snapshot omitted own accepted shader version")
	}
	if alice.OwnCode.Version != 7 || alice.OwnCode.Code != "// alice accepted" {
		t.Fatalf("wrong own code: %#v", alice.OwnCode)
	}
	if alice.Codes != nil {
		t.Fatalf("player snapshot leaked code map: %#v", alice.Codes)
	}

	unknown := g.Snapshot("unknown", false)
	if unknown.OwnCode != nil {
		t.Fatalf("unregistered player received code: %#v", unknown.OwnCode)
	}
}

func TestRecorderFailureDegradesRoundAndBlocksLaterMutation(t *testing.T) {
	recorder := &lifecycleRecorder{}
	g := New(recorder)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	recorder.appendErr = errors.New("disk full")
	if err := g.Register("u", "User"); err == nil {
		t.Fatal("failure accepted")
	}
	state := g.Snapshot("", true)
	if !state.Degraded || state.StorageError == "" {
		t.Fatalf("degraded state missing: %#v", state)
	}
	recorder.appendErr = nil
	if err := g.Register("u", "User"); err == nil {
		t.Fatal("later mutation accepted after degradation")
	}
}

func TestRecorderFailureIsObservable(t *testing.T) {
	recorder := &lifecycleRecorder{}
	g := New(recorder)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	recorder.appendErr = errors.New("disk full")
	if err := g.Register("u", "User"); err == nil {
		t.Fatal("recorder failure was silently accepted")
	}
	if got := g.Snapshot("u", false).Registrations; len(got) != 0 {
		t.Fatalf("failed registration remained in memory: %#v", got)
	}
}

func TestAdminRecordsRenderSnapshotManifest(t *testing.T) {
	g := New(nil)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "User"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	snapshot := RenderSnapshot{ID: "shot-1", TextureID: "n0:0", Format: "image/png", Width: 320, Height: 180, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 100, TimeMS: 42}
	if err := g.RecordRenderSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	state := g.Snapshot("", true)
	if len(state.RenderSnapshots) != 1 || state.RenderSnapshots[0].ID != "shot-1" {
		t.Fatalf("snapshot missing: %#v", state.RenderSnapshots)
	}
}

func TestAdminPublishesVersionedTimeControl(t *testing.T) {
	g := New(nil)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "User"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	if err := g.SetTimeControl(TimeControl{Version: 1, Action: "pause", TimeMS: 1200}); err != nil {
		t.Fatal(err)
	}
	state := g.Snapshot("", true)
	if state.TimeControl == nil || state.TimeControl.Action != "pause" || state.TimeControl.TimeMS != 1200 {
		t.Fatalf("bad control: %#v", state.TimeControl)
	}
	if err := g.SetTimeControl(TimeControl{Version: 1, Action: "resume", TimeMS: 1200}); err == nil {
		t.Fatal("stale control accepted")
	}
	if err := g.SetTimeControl(TimeControl{Version: 2, Action: "reset", TimeMS: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestAdminPublishesVersionedActiveTrajectoryToPlayers(t *testing.T) {
	g := New(nil)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "User"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	trajectory := Trajectory{Version: 1, Name: "Orbit", InitSource: "let radius=2;", FrameSource: "fov=60;", ValidatorVersion: "camera-v1", Active: true}
	if err := g.PublishTrajectory(trajectory); err != nil {
		t.Fatal(err)
	}
	player := g.Snapshot("u", false)
	if player.Trajectory == nil || player.Trajectory.Version != 1 || player.Trajectory.FrameSource != "fov=60;" {
		t.Fatalf("player did not receive active trajectory: %#v", player.Trajectory)
	}
	if err := g.PublishTrajectory(trajectory); err == nil {
		t.Fatal("stale trajectory version accepted")
	}
	trajectory.Version = 2
	trajectory.Active = false
	if err := g.PublishTrajectory(trajectory); err != nil {
		t.Fatal(err)
	}
	if got := g.Snapshot("u", false).Trajectory; got != nil {
		t.Fatalf("inactive trajectory projected to player: %#v", got)
	}
}

func TestTerminateClosesRoundJournal(t *testing.T) {
	recorder := &lifecycleRecorder{}
	g := New(recorder)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "User"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	if err := g.Terminate(); err != nil {
		t.Fatal(err)
	}
	if recorder.starts != 1 || recorder.closes != 1 {
		t.Fatalf("journal lifecycle starts=%d closes=%d", recorder.starts, recorder.closes)
	}
}

func TestRoundLifecycleAndVersion(t *testing.T) {
	g := New(nil)
	s := DefaultSettings()
	s.Players = 1
	if err := g.Configure(s); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "User"); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(); err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	if err := g.Upload("u", 2, "// shader"); err != nil {
		t.Fatal(err)
	}
	if err := g.Upload("u", 1, "old"); err == nil {
		t.Fatal("stale code accepted")
	}
	if err := g.Terminate(); err != nil {
		t.Fatal(err)
	}
}
