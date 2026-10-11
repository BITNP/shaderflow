package game

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const PresenceGrace = 30 * time.Second

// Settings describes the physical canvas controlled by the administrator. Every
// active player receives a square inside this canvas.
type Settings struct {
	Width        int `json:"width"`
	Height       int `json:"height"`
	MaxCodeBytes int `json:"maxCodeBytes"`
}

func DefaultSettings() Settings { return Settings{Width: 1920, Height: 1080, MaxCodeBytes: 65536} }
func (s Settings) Validate() error {
	if s.Width < 200 || s.Width > 16384 || s.Height < 200 || s.Height > 16384 {
		return errors.New("canvas dimensions must be between 200 and 16384")
	}
	if s.MaxCodeBytes < 1 || s.MaxCodeBytes > 1048576 {
		return errors.New("invalid code limit")
	}
	return nil
}

type Code struct {
	Version   int64     `json:"version"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Candidate struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
	Code Code   `json:"code"`
}

type Tile struct {
	Row    int `json:"row"`
	Column int `json:"column"`
	X      int `json:"x"`
	Y      int `json:"y"`
	Size   int `json:"size"`
}

type Player struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
	Tile Tile   `json:"tile"`
	Code Code   `json:"code,omitempty"`
}

type Layout struct {
	Rows     int `json:"rows"`
	Columns  int `json:"columns"`
	TileSize int `json:"tileSize"`
}

type State struct {
	Settings     Settings    `json:"settings"`
	Layout       Layout      `json:"layout"`
	Players      []Player    `json:"players,omitempty"`
	Candidates   []Candidate `json:"candidates,omitempty"`
	OwnCode      *Code       `json:"ownCode,omitempty"`
	OwnTile      *Tile       `json:"ownTile,omitempty"`
	OwnCandidate *Code       `json:"ownCandidate,omitempty"`
	Seq          uint64      `json:"seq"`
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

type Game struct {
	mu          sync.Mutex
	settings    Settings
	accepted    map[string]Candidate
	candidates  map[string]Candidate
	active      map[string]bool
	connections map[string]int
	timers      map[string]*time.Timer
	layout      Layout
	tiles       map[string]Tile
	seq         uint64
	events      []Envelope
	subscribers map[chan Envelope]struct{}
}

func New() *Game {
	return &Game{settings: DefaultSettings(), accepted: map[string]Candidate{}, candidates: map[string]Candidate{}, active: map[string]bool{}, connections: map[string]int{}, timers: map[string]*time.Timer{}, tiles: map[string]Tile{}, subscribers: map[chan Envelope]struct{}{}}
}

func (g *Game) emit(kind string, data any) {
	g.seq++
	e := Envelope{Type: "event", Seq: g.seq, Event: &Event{Type: kind, At: time.Now().UTC(), Data: data}}
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
}

func (g *Game) activePlayers() []Player {
	items := make([]Player, 0, len(g.accepted))
	for sub, candidate := range g.accepted {
		if !g.active[sub] {
			continue
		}
		items = append(items, Player{Sub: sub, Name: candidate.Name, Tile: g.tiles[sub], Code: candidate.Code})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sub < items[j].Sub })
	return items
}
func (g *Game) snapshot(sub string, admin bool) State {
	s := State{Settings: g.settings, Layout: g.layout, Seq: g.seq}
	if admin {
		s.Players = g.activePlayers()
		s.Candidates = make([]Candidate, 0, len(g.candidates))
		for _, candidate := range g.candidates {
			s.Candidates = append(s.Candidates, candidate)
		}
		sort.Slice(s.Candidates, func(i, j int) bool { return s.Candidates[i].Sub < s.Candidates[j].Sub })
		return s
	}
	if accepted, ok := g.accepted[sub]; ok {
		code := accepted.Code
		s.OwnCode = &code
	}
	if candidate, ok := g.candidates[sub]; ok {
		code := candidate.Code
		s.OwnCandidate = &code
	}
	if g.active[sub] {
		if tile, ok := g.tiles[sub]; ok {
			t := tile
			s.OwnTile = &t
		}
	}
	return s
}
func (g *Game) Snapshot(sub string, admin bool) State {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshot(sub, admin)
}

// SubmitCandidate persists nothing as accepted. Only an administrator can call
// Promote after its authoritative WebGPU renderer compiled and drew the code.
func (g *Game) SubmitCandidate(sub, name string, version int64, source string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if sub == "" || strings.TrimSpace(name) == "" {
		return errors.New("authenticated user required")
	}
	if version < 1 || strings.TrimSpace(source) == "" || len(source) > g.settings.MaxCodeBytes {
		return errors.New("invalid candidate code or version")
	}
	if err := ValidateSourceCharacters(source); err != nil {
		return err
	}
	if current, ok := g.candidates[sub]; ok && version <= current.Code.Version {
		return errors.New("stale candidate version")
	}
	if current, ok := g.accepted[sub]; ok && version <= current.Code.Version {
		return errors.New("candidate version must exceed accepted version")
	}
	g.touchLocked(sub)
	candidate := Candidate{Sub: sub, Name: name, Code: Code{Version: version, Source: source, UpdatedAt: time.Now().UTC()}}
	g.candidates[sub] = candidate
	g.emit("candidate.submitted", candidate)
	return nil
}
func (g *Game) Promote(sub string, version int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	candidate, ok := g.candidates[sub]
	if !ok {
		return errors.New("candidate not found")
	}
	if candidate.Code.Version != version {
		return errors.New("candidate version changed before promotion")
	}
	if current, ok := g.accepted[sub]; ok && version <= current.Code.Version {
		return errors.New("stale candidate version")
	}
	delete(g.candidates, sub)
	g.accepted[sub] = candidate
	if !g.active[sub] {
		g.active[sub] = true
		if g.connections[sub] == 0 {
			g.scheduleExpiryLocked(sub)
		}
	}
	g.addToLayoutLocked(sub)
	g.emit("code.accepted", Player{Sub: sub, Name: candidate.Name, Tile: g.tiles[sub], Code: candidate.Code})
	return nil
}
func (g *Game) UpdateSettings(next Settings) error {
	if err := next.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.settings = next
	g.rebuildLayoutLocked()
	g.emit("wall.resized", next)
	return nil
}

func (g *Game) touchLocked(sub string) {
	if timer := g.timers[sub]; timer != nil {
		timer.Stop()
		delete(g.timers, sub)
	}
	g.active[sub] = true
	if _, ok := g.accepted[sub]; ok {
		g.addToLayoutLocked(sub)
	}
	if g.connections[sub] == 0 {
		g.scheduleExpiryLocked(sub)
	}
}
func (g *Game) scheduleExpiryLocked(sub string) {
	g.timers[sub] = time.AfterFunc(PresenceGrace, func() { g.expire(sub) })
}
func (g *Game) expire(sub string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.connections[sub] > 0 {
		return
	}
	delete(g.timers, sub)
	if !g.active[sub] {
		return
	}
	delete(g.active, sub)
	delete(g.tiles, sub)
	if _, accepted := g.accepted[sub]; accepted {
		g.rebuildIfSparseLocked()
		g.emit("player.offline", map[string]string{"sub": sub})
	}
}
func (g *Game) addToLayoutLocked(sub string) {
	if _, ok := g.tiles[sub]; ok {
		return
	}
	capacity := g.layout.Rows * g.layout.Columns
	if capacity == 0 || len(g.tiles) >= capacity {
		g.rebuildLayoutLocked()
		return
	}
	for row := 0; row < g.layout.Rows; row++ {
		for col := 0; col < g.layout.Columns; col++ {
			used := false
			for _, tile := range g.tiles {
				if tile.Row == row && tile.Column == col {
					used = true
					break
				}
			}
			if !used {
				g.tiles[sub] = g.makeTile(row, col)
				return
			}
		}
	}
	g.rebuildLayoutLocked()
}
func (g *Game) rebuildIfSparseLocked() {
	capacity := g.layout.Rows * g.layout.Columns
	if capacity > 0 && capacity-len(g.tiles) > capacity/4 {
		g.rebuildLayoutLocked()
	}
}
func (g *Game) makeTile(row, col int) Tile {
	side := g.layout.TileSize
	usedW := g.layout.Columns * side
	usedH := g.layout.Rows * side
	return Tile{Row: row, Column: col, X: (g.settings.Width-usedW)/2 + col*side, Y: (g.settings.Height-usedH)/2 + row*side, Size: side}
}
func (g *Game) rebuildLayoutLocked() {
	n := 0
	subs := make([]string, 0)
	for sub := range g.accepted {
		if g.active[sub] {
			n++
			subs = append(subs, sub)
		}
	}
	sort.Strings(subs)
	g.tiles = map[string]Tile{}
	if n == 0 {
		g.layout = Layout{}
		return
	}
	bestRows, bestCols, bestSide := 1, n, 0
	for cols := 1; cols <= n; cols++ {
		rows := (n + cols - 1) / cols
		side := min(g.settings.Width/cols, g.settings.Height/rows)
		if side > bestSide || side == bestSide && rows*cols < bestRows*bestCols {
			bestRows, bestCols, bestSide = rows, cols, side
		}
	}
	g.layout = Layout{Rows: bestRows, Columns: bestCols, TileSize: bestSide}
	for i, sub := range subs {
		g.tiles[sub] = g.makeTile(i/bestCols, i%bestCols)
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *Game) Subscribe(sub string, admin bool, since uint64) (chan Envelope, []Envelope, func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan Envelope, 128)
	g.subscribers[ch] = struct{}{}
	if !admin && sub != "" {
		g.connections[sub]++
		g.touchLocked(sub)
	}
	initial := []Envelope{}
	if since == 0 || since > g.seq || (len(g.events) > 0 && since < g.events[0].Seq-1) {
		s := g.snapshot(sub, admin)
		initial = []Envelope{{Type: "snapshot", Seq: g.seq, State: &s}}
	} else {
		for _, event := range g.events {
			if event.Seq > since {
				initial = append(initial, g.filter(event, sub, admin))
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
		if !admin && sub != "" {
			g.connections[sub]--
			if g.connections[sub] <= 0 {
				delete(g.connections, sub)
				g.scheduleExpiryLocked(sub)
			}
		}
	}
	return ch, initial, cancel
}
func (g *Game) filter(event Envelope, sub string, admin bool) Envelope {
	if admin {
		return event
	}
	s := g.snapshot(sub, false)
	return Envelope{Type: "snapshot", Seq: event.Seq, State: &s}
}
func (g *Game) Visible(event Envelope, sub string, admin bool) Envelope {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.filter(event, sub, admin)
}

// WGSL program text is intentionally only a candidate payload here. The
// authoritative browser verifies the locked wrapper and WebGPU pipeline before
// promotion. This prevents a curl request from becoming displayed shader code.
func ValidateSourceCharacters(source string) error {
	if !utf8.ValidString(source) {
		return fmt.Errorf("shader source must be valid UTF-8")
	}
	return nil
}
