package game

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"
)

type Settings struct {
	Players           int    `json:"players"`
	InputCount        int    `json:"inputCount"`
	MinInputs         int    `json:"minInputs"`
	MaxInputs         int    `json:"maxInputs"`
	MinOutputs        int    `json:"minOutputs"`
	MaxOutputs        int    `json:"maxOutputs"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Format            string `json:"format"`
	BlockSize         int    `json:"blockSize"`
	MaxCodeBytes      int    `json:"maxCodeBytes"`
	CompileDebounceMs int    `json:"compileDebounceMs"`
	PassTimeoutMs     int    `json:"passTimeoutMs"`
}

func DefaultSettings() Settings {
	return Settings{Players: 2, InputCount: 1, MinInputs: 1, MaxInputs: 2, MinOutputs: 1, MaxOutputs: 2, Width: 960, Height: 540, Format: "rgba32float", BlockSize: 8, MaxCodeBytes: 65536, CompileDebounceMs: 500, PassTimeoutMs: 1000}
}
func (s Settings) Validate() error {
	if s.Players < 1 || s.Players > 10 || s.InputCount < 1 || s.InputCount > 32 || s.MinInputs < 1 || s.MaxInputs < s.MinInputs || s.MaxInputs > 32 || s.MinOutputs < 1 || s.MaxOutputs < s.MinOutputs || s.MaxOutputs > 32 || s.Width < 200 || s.Width > 3840 || s.Height < 100 || s.Height > 2160 || s.MaxCodeBytes < 1 || s.MaxCodeBytes > 1048576 || s.CompileDebounceMs < 1 || s.PassTimeoutMs < 1 {
		return errors.New("invalid settings ranges")
	}
	if s.Format != "rgba32float" && s.Format != "rgba16float" && s.Format != "rgba8unorm" {
		return errors.New("unsupported texture format")
	}
	if s.BlockSize != 4 && s.BlockSize != 8 && s.BlockSize != 16 {
		return errors.New("blockSize must be 4, 8, or 16")
	}
	return nil
}

type Node struct {
	ID      string   `json:"id"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`
}
type Graph struct {
	Inputs []string `json:"inputs"`
	Nodes  []Node   `json:"nodes"`
	Output string   `json:"output"`
}

// Validate requires every source/terminal to be connected to a path between an external input and the final output.
func (g Graph) Validate(players int) error {
	if len(g.Nodes) != players || len(g.Inputs) < 1 || g.Output == "" {
		return errors.New("graph node/input/output count invalid")
	}
	producer := map[string]string{}
	consumers := map[string]int{}
	ids := map[string]bool{}
	for _, src := range g.Inputs {
		if src == "" || producer[src] != "" {
			return errors.New("duplicate or empty external input")
		}
		producer[src] = "input"
	}
	for _, n := range g.Nodes {
		if n.ID == "" || ids[n.ID] || len(n.Inputs) < 1 || len(n.Outputs) < 1 {
			return errors.New("duplicate node or missing terminal")
		}
		ids[n.ID] = true
		for _, o := range n.Outputs {
			if o == "" || producer[o] != "" {
				return fmt.Errorf("duplicate output %q", o)
			}
			producer[o] = n.ID
		}
	}
	adj := map[string]map[string]bool{}
	indegree := map[string]int{}
	for _, n := range g.Nodes {
		for _, src := range n.Inputs {
			owner := producer[src]
			if owner == "" {
				return fmt.Errorf("unconnected input %q", src)
			}
			consumers[src]++
			if owner == n.ID {
				return errors.New("self cycle")
			}
			if owner != "input" {
				if adj[owner] == nil {
					adj[owner] = map[string]bool{}
				}
				if !adj[owner][n.ID] {
					adj[owner][n.ID] = true
					indegree[n.ID]++
				}
			}
		}
	}
	if producer[g.Output] == "" || producer[g.Output] == "input" {
		return errors.New("final output must come from shader")
	}
	consumers[g.Output]++
	for src := range producer {
		if consumers[src] == 0 {
			return fmt.Errorf("unused output %q", src)
		}
	}
	queue := []string{}
	for _, n := range g.Nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	count := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		count++
		for next := range adj[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if count != players {
		return errors.New("graph has cycle")
	}
	// Traverse backwards from the final output: all nodes and all external inputs must contribute.
	visited := map[string]bool{}
	var visit func(string)
	byID := map[string]Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	visit = func(src string) {
		if visited[src] {
			return
		}
		visited[src] = true
		if owner := producer[src]; owner != "input" {
			for _, up := range byID[owner].Inputs {
				visit(up)
			}
		}
	}
	visit(g.Output)
	for src := range producer {
		if !visited[src] {
			return fmt.Errorf("terminal %q cannot reach final output", src)
		}
	}
	return nil
}
func randomInt(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// Generate builds a connected chain and distributes additional external sources and outputs as branches.
func Generate(s Settings) (Graph, error) {
	if err := s.Validate(); err != nil {
		return Graph{}, err
	}
	g := Graph{Inputs: make([]string, s.InputCount), Nodes: make([]Node, s.Players)}
	for i := range g.Inputs {
		g.Inputs[i] = fmt.Sprintf("input%d", i)
	}
	// Every node needs its own output; the chain guarantees all nodes reach the final sink.
	for i := range g.Nodes {
		q := s.MinOutputs + randomInt(s.MaxOutputs-s.MinOutputs+1)
		if q < 1 {
			q = 1
		}
		n := Node{ID: fmt.Sprintf("n%d", i), Outputs: make([]string, q)}
		for j := range n.Outputs {
			n.Outputs[j] = fmt.Sprintf("n%d:%d", i, j)
		}
		g.Nodes[i] = n
	}
	for i := range g.Nodes {
		if i == 0 {
			g.Nodes[i].Inputs = append(g.Nodes[i].Inputs, g.Inputs[0])
		} else {
			g.Nodes[i].Inputs = append(g.Nodes[i].Inputs, g.Nodes[i-1].Outputs[0])
		}
	}
	g.Output = g.Nodes[len(g.Nodes)-1].Outputs[0]
	// All external images feed a shader on the chain.
	for i := 1; i < len(g.Inputs); i++ {
		target := randomInt(len(g.Nodes))
		g.Nodes[target].Inputs = append(g.Nodes[target].Inputs, g.Inputs[i])
	}
	// Extra outputs need a later consumer; last node uses same final source rather than dead terminals.
	for i := range g.Nodes {
		for j := 1; j < len(g.Nodes[i].Outputs); j++ {
			if i == len(g.Nodes)-1 {
				g.Nodes[i].Outputs = g.Nodes[i].Outputs[:j]
				break
			}
			target := i + 1 + randomInt(len(g.Nodes)-i-1)
			g.Nodes[target].Inputs = append(g.Nodes[target].Inputs, g.Nodes[i].Outputs[j])
		}
	}
	// Randomize the requested input arity, while existing connectivity may already consume part of it.
	for i := range g.Nodes {
		targetInputs := s.MinInputs + randomInt(s.MaxInputs-s.MinInputs+1)
		if targetInputs < len(g.Nodes[i].Inputs) {
			targetInputs = len(g.Nodes[i].Inputs)
		}
		for len(g.Nodes[i].Inputs) < targetInputs {
			if i == 0 {
				g.Nodes[i].Inputs = append(g.Nodes[i].Inputs, g.Inputs[randomInt(len(g.Inputs))])
			} else {
				j := randomInt(i)
				g.Nodes[i].Inputs = append(g.Nodes[i].Inputs, g.Nodes[j].Outputs[randomInt(len(g.Nodes[j].Outputs))])
			}
		}
	}
	return g, g.Validate(s.Players)
}

type Player struct {
	Sub    string `json:"sub"`
	Name   string `json:"name"`
	NodeID string `json:"nodeId,omitempty"`
	Times  int    `json:"times"`
}
type Code struct {
	Version   int64     `json:"version"`
	Code      string    `json:"code"`
	UpdatedAt time.Time `json:"updatedAt"`
	Sub       string    `json:"sub"`
}
type ExternalInput struct {
	ID          string    `json:"id"`
	RoundID     string    `json:"roundId"`
	Version     int64     `json:"version"`
	SHA256      string    `json:"sha256"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	ResizeMode  string    `json:"resizeMode"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type RenderSnapshot struct {
	ID                 string           `json:"id"`
	RoundID            string           `json:"roundId"`
	TextureID          string           `json:"textureId"`
	Format             string           `json:"format"`
	Width              int              `json:"width"`
	Height             int              `json:"height"`
	SHA256             string           `json:"sha256"`
	Size               int64            `json:"size"`
	TimeMS             float64          `json:"timeMs"`
	SourceSeq          uint64           `json:"sourceSeq"`
	CodeVersions       map[string]int64 `json:"codeVersions,omitempty"`
	InputVersions      map[string]int64 `json:"inputVersions,omitempty"`
	TrajectoryVersion  int64            `json:"trajectoryVersion,omitempty"`
	TimeControlVersion int64            `json:"timeControlVersion,omitempty"`
	EventSeq           uint64           `json:"eventSeq"`
	CreatedAt          time.Time        `json:"createdAt"`
}
type TimeControl struct {
	Version   int64     `json:"version"`
	Action    string    `json:"action"`
	TimeMS    float64   `json:"timeMs"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Trajectory struct {
	Version          int64     `json:"version"`
	Name             string    `json:"name"`
	InitSource       string    `json:"initSource"`
	FrameSource      string    `json:"frameSource"`
	ValidatorVersion string    `json:"validatorVersion"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Active           bool      `json:"active"`
}
type State struct {
	Phase           string                   `json:"phase"`
	Settings        Settings                 `json:"settings"`
	Graph           *Graph                   `json:"graph,omitempty"`
	Registrations   []Player                 `json:"registrations"`
	Players         []Player                 `json:"players,omitempty"`
	OwnNode         *Node                    `json:"ownNode,omitempty"`
	OwnCode         *Code                    `json:"ownCode,omitempty"`
	Trajectory      *Trajectory              `json:"trajectory,omitempty"`
	TimeControl     *TimeControl             `json:"timeControl,omitempty"`
	ExternalInputs  map[string]ExternalInput `json:"externalInputs,omitempty"`
	RenderSnapshots []RenderSnapshot         `json:"renderSnapshots,omitempty"`
	Codes           map[string]Code          `json:"codes,omitempty"`
	Seq             uint64                   `json:"seq"`
	Degraded        bool                     `json:"degraded,omitempty"`
	StorageError    string                   `json:"storageError,omitempty"`
}
type Event struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	Data any       `json:"data,omitempty"`
}
type Envelope struct {
	Type  string `json:"type"`
	Seq   uint64 `json:"seq"`
	State *State `json:"state,omitempty"`
	Event *Event `json:"event,omitempty"`
}
type Recorder interface {
	Start(State) error
	Append(Envelope) error
	Code(string, Code) error
}

type RoundJournal interface {
	Recorder
	Close() error
}
type Game struct {
	mu              sync.Mutex
	phase           string
	settings        Settings
	graph           *Graph
	registrations   []Player
	history         map[string]int
	codes           map[string]Code
	externalInputs  map[string]ExternalInput
	trajectory      *Trajectory
	timeControl     *TimeControl
	renderSnapshots []RenderSnapshot
	seq             uint64
	events          []Envelope
	subscribers     map[chan Envelope]struct{}
	recorder        Recorder
	storageErr      error
}

func New(rec Recorder) *Game {
	return &Game{phase: "idle", settings: DefaultSettings(), history: map[string]int{}, codes: map[string]Code{}, externalInputs: map[string]ExternalInput{}, subscribers: map[chan Envelope]struct{}{}, recorder: rec}
}
func (g *Game) snapshot(sub string, admin bool) State {
	s := State{Phase: g.phase, Settings: g.settings, Seq: g.seq, Registrations: []Player{}}
	if g.storageErr != nil {
		s.Degraded = true
		s.StorageError = g.storageErr.Error()
	}
	if g.timeControl != nil {
		control := *g.timeControl
		s.TimeControl = &control
	}
	if g.trajectory != nil && (admin || g.trajectory.Active) {
		trajectory := *g.trajectory
		s.Trajectory = &trajectory
	}
	if admin && len(g.renderSnapshots) > 0 {
		s.RenderSnapshots = append([]RenderSnapshot(nil), g.renderSnapshots...)
	}
	if admin && len(g.externalInputs) > 0 {
		s.ExternalInputs = map[string]ExternalInput{}
		for id, input := range g.externalInputs {
			s.ExternalInputs[id] = input
		}
	}
	if admin {
		s.Graph = g.graph
		s.Registrations = append(s.Registrations, g.registrations...)
		s.Players = append([]Player{}, g.registrations...)
		s.Codes = map[string]Code{}
		for k, v := range g.codes {
			s.Codes[k] = v
		}
	} else {
		for _, p := range g.registrations {
			if p.Sub == sub {
				s.Registrations = append(s.Registrations, p)
				if g.phase == "running" && g.graph != nil {
					for _, n := range g.graph.Nodes {
						if n.ID == p.NodeID {
							nCopy := n
							s.OwnNode = &nCopy
							if code, ok := g.codes[n.ID]; ok && code.Sub == sub {
								codeCopy := code
								s.OwnCode = &codeCopy
							}
						}
					}
				}
				break
			}
		}
	}
	return s
}
func (g *Game) Snapshot(sub string, admin bool) State {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshot(sub, admin)
}
func (g *Game) emit(t string, data any) error {
	if g.storageErr != nil {
		return fmt.Errorf("round storage degraded: %w", g.storageErr)
	}
	g.seq++
	e := Envelope{Type: "event", Seq: g.seq, Event: &Event{Type: t, At: time.Now().UTC(), Data: data}}
	if g.recorder != nil {
		if err := g.recorder.Append(e); err != nil {
			g.seq--
			g.storageErr = err
			return fmt.Errorf("record %s: %w", t, err)
		}
	}
	g.events = append(g.events, e)
	if len(g.events) > 4096 {
		g.events = g.events[len(g.events)-4096:]
	}
	for ch := range g.subscribers {
		select {
		case ch <- e:
		default:
			close(ch)
			delete(g.subscribers, ch)
		}
	}
	return nil
}
func (g *Game) Subscribe(sub string, admin bool, since uint64) (chan Envelope, []Envelope, func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan Envelope, 128)
	g.subscribers[ch] = struct{}{}
	initial := []Envelope{}
	if since == 0 || since > g.seq || (len(g.events) > 0 && since < g.events[0].Seq-1) {
		s := g.snapshot(sub, admin)
		initial = append(initial, Envelope{Type: "snapshot", Seq: g.seq, State: &s})
	} else {
		for _, e := range g.events {
			if e.Seq > since {
				initial = append(initial, g.filter(e, sub, admin))
			}
		}
	}
	cancel := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if _, ok := g.subscribers[ch]; ok {
			delete(g.subscribers, ch)
			close(ch)
		}
	}
	return ch, initial, cancel
}
func (g *Game) filter(e Envelope, sub string, admin bool) Envelope {
	if admin {
		return e
	}
	s := g.snapshot(sub, false)
	return Envelope{Type: "snapshot", Seq: e.Seq, State: &s}
}
func (g *Game) Visible(e Envelope, sub string, admin bool) Envelope {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.filter(e, sub, admin)
}
func (g *Game) Configure(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase == "running" {
		return errors.New("round already running")
	}
	previous := struct {
		settings      Settings
		phase         string
		graph         *Graph
		registrations []Player
		codes         map[string]Code
	}{g.settings, g.phase, g.graph, g.registrations, g.codes}
	g.settings = s
	g.phase = "registration"
	g.graph = nil
	g.registrations = nil
	g.codes = map[string]Code{}
	if err := g.emit("round.configured", s); err != nil {
		g.settings, g.phase, g.graph, g.registrations, g.codes = previous.settings, previous.phase, previous.graph, previous.registrations, previous.codes
		return err
	}
	return nil
}
func (g *Game) Register(sub, name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "registration" && g.phase != "graph" {
		return errors.New("registration closed")
	}
	for _, p := range g.registrations {
		if p.Sub == sub {
			return nil
		}
	}
	if len(g.registrations) >= g.settings.Players {
		return errors.New("registration full")
	}
	p := Player{Sub: sub, Name: name, Times: g.history[sub]}
	g.registrations = append(g.registrations, p)
	if err := g.emit("player.registered", p); err != nil {
		g.registrations = g.registrations[:len(g.registrations)-1]
		return err
	}
	return nil
}
func (g *Game) Remove(sub string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "registration" && g.phase != "graph" {
		return errors.New("cannot remove after start")
	}
	for i, p := range g.registrations {
		if p.Sub == sub {
			g.registrations = append(g.registrations[:i], g.registrations[i+1:]...)
			g.graph = nil
			g.phase = "registration"
			g.emit("player.removed", sub)
			return nil
		}
	}
	return errors.New("registration not found")
}
func (g *Game) Generate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "registration" && g.phase != "graph" {
		return errors.New("wrong phase")
	}
	if len(g.registrations) != g.settings.Players {
		return errors.New("registration not full")
	}
	graph, err := Generate(g.settings)
	if err != nil {
		return err
	}
	previousGraph, previousPhase := g.graph, g.phase
	g.graph = &graph
	g.phase = "graph"
	if err := g.emit("graph.updated", graph); err != nil {
		g.graph, g.phase = previousGraph, previousPhase
		return err
	}
	return nil
}
func (g *Game) SetGraph(graph Graph) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "graph" {
		return errors.New("graph editing not available")
	}
	if len(graph.Inputs) > 32 {
		return errors.New("too many external inputs")
	}
	if err := graph.Validate(g.settings.Players); err != nil {
		return err
	}
	previousGraph, previousInputCount := g.graph, g.settings.InputCount
	g.graph = &graph
	g.settings.InputCount = len(graph.Inputs)
	if err := g.emit("graph.updated", graph); err != nil {
		g.graph, g.settings.InputCount = previousGraph, previousInputCount
		return err
	}
	return nil
}
func (g *Game) Start() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "graph" || g.graph == nil || len(g.registrations) != g.settings.Players {
		return errors.New("graph or registrations incomplete")
	}
	if err := g.graph.Validate(g.settings.Players); err != nil {
		return err
	}
	perm := append([]Player(nil), g.registrations...)
	for i := len(perm) - 1; i > 0; i-- {
		j := randomInt(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	for i := range perm {
		perm[i].NodeID = g.graph.Nodes[i].ID
		perm[i].Times = g.history[perm[i].Sub] + 1
	}
	sort.Slice(perm, func(i, j int) bool { return perm[i].NodeID < perm[j].NodeID })
	previous := g.registrations
	g.registrations = perm
	g.phase = "running"
	if g.recorder != nil {
		if err := g.recorder.Start(g.snapshot("", true)); err != nil {
			g.registrations = previous
			g.phase = "graph"
			return err
		}
	}
	if err := g.emit("round.started", g.snapshot("", true)); err != nil {
		if journal, ok := g.recorder.(RoundJournal); ok {
			_ = journal.Close()
		}
		g.registrations, g.phase = previous, "graph"
		return err
	}
	for _, player := range perm {
		g.history[player.Sub] = player.Times
	}
	return nil
}
func (g *Game) Terminate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" {
		return errors.New("round not running")
	}
	if err := g.emit("round.terminated", nil); err != nil {
		return err
	}
	if journal, ok := g.recorder.(RoundJournal); ok {
		if err := journal.Close(); err != nil {
			return err
		}
	}
	g.phase = "registration"
	g.graph = nil
	g.codes = map[string]Code{}
	g.externalInputs = map[string]ExternalInput{}
	g.trajectory = nil
	g.timeControl = nil
	g.renderSnapshots = nil
	for i := range g.registrations {
		g.registrations[i].NodeID = ""
	}
	if err := g.emit("round.reset", nil); err != nil {
		return err
	}
	return nil
}
func (g *Game) RecordRenderSnapshot(snapshot RenderSnapshot) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" || g.graph == nil {
		return errors.New("round not running")
	}
	validTexture := false
	for _, id := range g.graph.Inputs {
		if id == snapshot.TextureID {
			validTexture = true
		}
	}
	for _, node := range g.graph.Nodes {
		for _, id := range node.Outputs {
			if id == snapshot.TextureID {
				validTexture = true
			}
		}
	}
	if !validTexture {
		return errors.New("unknown snapshot texture")
	}
	if snapshot.ID == "" || snapshot.Width < 1 || snapshot.Height < 1 || snapshot.Size < 1 || len(snapshot.SHA256) != 64 {
		return errors.New("invalid render snapshot")
	}
	if snapshot.Format != "image/png" && snapshot.Format != "image/jpeg" {
		return errors.New("unsupported snapshot format")
	}
	for _, existing := range g.renderSnapshots {
		if existing.ID == snapshot.ID {
			return errors.New("duplicate render snapshot")
		}
	}
	snapshot.CreatedAt = time.Now().UTC()
	snapshot.EventSeq = g.seq + 1
	g.renderSnapshots = append(g.renderSnapshots, snapshot)
	if err := g.emit("snapshot.created", snapshot); err != nil {
		g.renderSnapshots = g.renderSnapshots[:len(g.renderSnapshots)-1]
		return err
	}
	return nil
}

func (g *Game) SetTimeControl(next TimeControl) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" {
		return errors.New("round not running")
	}
	if next.Version < 1 || !map[string]bool{"pause": true, "resume": true, "reset": true}[next.Action] || next.TimeMS < 0 {
		return errors.New("invalid time control")
	}
	if g.timeControl != nil && next.Version <= g.timeControl.Version {
		return errors.New("stale time control version")
	}
	next.UpdatedAt = time.Now().UTC()
	previous := g.timeControl
	g.timeControl = &next
	if err := g.emit("time.controlled", next); err != nil {
		g.timeControl = previous
		return err
	}
	return nil
}

func (g *Game) PublishExternalInput(next ExternalInput) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" || g.graph == nil {
		return errors.New("round not running")
	}
	valid := false
	for _, id := range g.graph.Inputs {
		if id == next.ID {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("unknown external input")
	}
	if next.Version < 1 || len(next.SHA256) != 64 || next.Size < 1 {
		return errors.New("invalid external input reference")
	}
	if next.ContentType != "image/png" && next.ContentType != "image/jpeg" {
		return errors.New("unsupported input content type")
	}
	if !map[string]bool{"stretch": true, "cover": true, "contain": true, "transparent": true}[next.ResizeMode] {
		return errors.New("invalid resize mode")
	}
	if previous, ok := g.externalInputs[next.ID]; ok && next.Version <= previous.Version {
		return errors.New("stale external input version")
	}
	next.UpdatedAt = time.Now().UTC()
	previous, hadPrevious := g.externalInputs[next.ID]
	g.externalInputs[next.ID] = next
	if err := g.emit("input.updated", next); err != nil {
		if hadPrevious {
			g.externalInputs[next.ID] = previous
		} else {
			delete(g.externalInputs, next.ID)
		}
		return err
	}
	return nil
}

func (g *Game) PublishTrajectory(next Trajectory) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" {
		return errors.New("round not running")
	}
	if next.Version < 1 || len(next.Name) == 0 || len(next.Name) > 128 {
		return errors.New("invalid trajectory name or version")
	}
	if len(next.InitSource)+len(next.FrameSource) > 65536 {
		return errors.New("trajectory too large")
	}
	if next.ValidatorVersion == "" {
		return errors.New("validator version required")
	}
	if g.trajectory != nil && next.Version <= g.trajectory.Version {
		return errors.New("stale trajectory version")
	}
	next.UpdatedAt = time.Now().UTC()
	previous := g.trajectory
	g.trajectory = &next
	if err := g.emit("trajectory.updated", next); err != nil {
		g.trajectory = previous
		return err
	}
	return nil
}

func ValidateSourceCharacters(source string) error {
	// WGSL tokens must be ASCII; comments and quoted text may contain UTF-8.
	mode := byte(0) // 0 code, 1 line comment, 2 block comment, 3 string
	for i := 0; i < len(source); i++ {
		c := source[i]
		if mode == 1 {
			if c == '\n' {
				mode = 0
			}
			continue
		}
		if mode == 2 {
			if c == '*' && i+1 < len(source) && source[i+1] == '/' {
				mode = 0
				i++
			}
			continue
		}
		if mode == 3 {
			if c == '\\' && i+1 < len(source) {
				i++
				continue
			}
			if c == '"' {
				mode = 0
			}
			continue
		}
		if c == '/' && i+1 < len(source) {
			if source[i+1] == '/' {
				mode = 1
				i++
				continue
			}
			if source[i+1] == '*' {
				mode = 2
				i++
				continue
			}
		}
		if c == '"' {
			mode = 3
			continue
		}
		if c >= 128 {
			return errors.New("non-ASCII WGSL token")
		}
	}
	return nil
}
func (g *Game) Upload(sub string, version int64, source string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "running" {
		return errors.New("round not running")
	}
	if version < 1 || len(source) == 0 || len(source) > g.settings.MaxCodeBytes {
		return errors.New("invalid code length or version")
	}
	if err := ValidateSourceCharacters(source); err != nil {
		return err
	}
	node := ""
	for _, p := range g.registrations {
		if p.Sub == sub {
			node = p.NodeID
			break
		}
	}
	if node == "" {
		return errors.New("player not assigned")
	}
	if previous, ok := g.codes[node]; ok && version <= previous.Version {
		return errors.New("stale client version")
	}
	c := Code{Version: version, Code: source, UpdatedAt: time.Now().UTC(), Sub: sub}
	if g.recorder != nil {
		if err := g.recorder.Code(node, c); err != nil {
			return err
		}
	}
	previous, hadPrevious := g.codes[node]
	g.codes[node] = c
	if err := g.emit("code.updated", map[string]any{"nodeId": node, "code": c}); err != nil {
		if hadPrevious {
			g.codes[node] = previous
		} else {
			delete(g.codes, node)
		}
		return err
	}
	return nil
}
