package game

import "testing"

func TestCandidateRequiresPromotionBeforeItBecomesVisible(t *testing.T) {
	g := New()
	if err := g.SubmitCandidate("alice", "Alice", 1, "const positiveColor = vec3f(1.);\nconst negativeColor = vec3f(0.);\nfn userfn(x: f32, y: f32, t: f32) -> f32 { return x; }"); err != nil {
		t.Fatal(err)
	}
	if got := g.Snapshot("", true); len(got.Players) != 0 || len(got.Candidates) != 1 {
		t.Fatalf("candidate was treated as active: %#v", got)
	}
	if err := g.Promote("alice", 1); err != nil {
		t.Fatal(err)
	}
	state := g.Snapshot("", true)
	if len(state.Players) != 1 || state.Players[0].Code.Version != 1 || state.Players[0].Tile.Size == 0 {
		t.Fatalf("promotion did not create tile: %#v", state)
	}
	player := g.Snapshot("alice", false)
	if player.OwnCode == nil || player.OwnCandidate != nil || player.OwnTile == nil {
		t.Fatalf("player snapshot incorrect: %#v", player)
	}
}

func TestKickRemovesPlayerUntilHigherVersionIsPromoted(t *testing.T) {
	g := New()
	if err := g.SubmitCandidate("alice", "Alice", 1, "one"); err != nil {
		t.Fatal(err)
	}
	if err := g.Promote("alice", 1); err != nil {
		t.Fatal(err)
	}
	if err := g.Kick("alice"); err != nil {
		t.Fatal(err)
	}
	if got := g.Snapshot("", true).Players; len(got) != 0 {
		t.Fatalf("kicked player still active: %#v", got)
	}
	if err := g.SubmitCandidate("alice", "Alice", 1, "old"); err == nil {
		t.Fatal("kicked player reused old version")
	}
	if err := g.SubmitCandidate("alice", "Alice", 2, "fixed"); err != nil {
		t.Fatal(err)
	}
	if got := g.Snapshot("", true).Players; len(got) != 0 {
		t.Fatalf("old code returned before promotion: %#v", got)
	}
	if err := g.Promote("alice", 2); err != nil {
		t.Fatal(err)
	}
	if got := g.Snapshot("", true).Players; len(got) != 1 || got[0].Code.Version != 2 {
		t.Fatalf("higher version did not restore player: %#v", got)
	}
}

func TestMetricsAttachOnlyToActiveCurrentCode(t *testing.T) {
	g := New()
	if err := g.SubmitCandidate("alice", "Alice", 1, "one"); err != nil {
		t.Fatal(err)
	}
	if err := g.Promote("alice", 1); err != nil {
		t.Fatal(err)
	}
	gpu := 4.5
	g.RecordMetrics([]MetricReport{{Sub: "alice", Version: 1, SubmitMS: 0.2, GPUMS: &gpu}})
	player := g.Snapshot("", true).Players[0]
	if player.Metrics == nil || player.Metrics.GPUMS == nil || *player.Metrics.GPUMS != gpu || player.Metrics.SubmitMS != 0.2 {
		t.Fatalf("metrics missing: %#v", player)
	}
	g.RecordMetrics([]MetricReport{{Sub: "alice", Version: 2, SubmitMS: 99}})
	if got := g.Snapshot("", true).Players[0].Metrics; got == nil || got.Version != 1 {
		t.Fatalf("stale metric replaced current one: %#v", got)
	}
}

func TestCandidateVersionMustAdvance(t *testing.T) {
	g := New()
	if err := g.SubmitCandidate("alice", "Alice", 3, "ok"); err != nil {
		t.Fatal(err)
	}
	if err := g.SubmitCandidate("alice", "Alice", 3, "different"); err == nil {
		t.Fatal("stale candidate accepted")
	}
	if err := g.Promote("alice", 3); err != nil {
		t.Fatal(err)
	}
	if err := g.SubmitCandidate("alice", "Alice", 3, "again"); err == nil {
		t.Fatal("candidate did not advance beyond accepted code")
	}
}

func TestLayoutUsesLargestCenteredSquares(t *testing.T) {
	g := New()
	if err := g.UpdateViewport(1000, 600); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"a", "b", "c"} {
		if err := g.SubmitCandidate(sub, sub, 1, "x"); err != nil {
			t.Fatal(err)
		}
		if err := g.Promote(sub, 1); err != nil {
			t.Fatal(err)
		}
	}
	state := g.Snapshot("", true)
	if state.Layout.Rows*state.Layout.Columns < 3 || state.Layout.TileSize != 333 {
		t.Fatalf("unexpected layout: %#v", state.Layout)
	}
	for _, player := range state.Players {
		if player.Tile.X < 0 || player.Tile.Y < 0 || player.Tile.Size != state.Layout.TileSize {
			t.Fatalf("bad tile: %#v", player.Tile)
		}
	}
}
