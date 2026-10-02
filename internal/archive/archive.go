package archive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"shaderflow/internal/game"
)

const SchemaVersion = 1
const CheckpointInterval uint64 = 50

type Manifest struct {
	SchemaVersion   int       `json:"schemaVersion"`
	RoundID         string    `json:"roundId"`
	StartedAt       time.Time `json:"startedAt"`
	ClosedAt        time.Time `json:"closedAt,omitempty"`
	Status          string    `json:"status"`
	EventCount      uint64    `json:"eventCount,omitempty"`
	CheckpointCount uint64    `json:"checkpointCount,omitempty"`
	BlobCount       uint64    `json:"blobCount,omitempty"`
	BlobBytes       int64     `json:"blobBytes,omitempty"`
	DegradedReason  string    `json:"degradedReason,omitempty"`
}

type Round struct {
	Manifest    Manifest        `json:"manifest"`
	Start       game.State      `json:"start"`
	Events      []game.Envelope `json:"events"`
	Checkpoints []Checkpoint    `json:"checkpoints,omitempty"`
}
type BlobRef struct {
	RoundID     string `json:"roundId"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
}

type Status struct {
	Ready bool   `json:"ready"`
	Error string `json:"error,omitempty"`
}
type Filesystem struct {
	mu         sync.Mutex
	root       string
	round      string
	manifest   Manifest
	state      game.State
	degraded   error
	failWrites error
}

func NewFilesystem(root string) *Filesystem { return &Filesystem{root: root} }
func (f *Filesystem) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.degraded != nil {
		return Status{Error: f.degraded.Error()}
	}
	return Status{Ready: true}
}
func (f *Filesystem) fail(err error) error {
	if err != nil && f.degraded == nil {
		f.degraded = err
	}
	return err
}
func (f *Filesystem) writable() error {
	if f.degraded != nil {
		return fmt.Errorf("archive degraded: %w", f.degraded)
	}
	if f.failWrites != nil {
		return f.fail(f.failWrites)
	}
	return nil
}
func (f *Filesystem) Health() error {
	if err := os.MkdirAll(f.root, 0700); err != nil {
		return err
	}
	path := filepath.Join(f.root, ".shaderflow-health")
	if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
		return err
	}
	return os.Remove(path)
}

func (f *Filesystem) Start(state game.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(); err != nil {
		return err
	}
	if f.round != "" {
		return errors.New("round archive already active")
	}
	now := time.Now().UTC()
	id := now.Format("20060102-150405.000000000")
	dir := filepath.Join(f.root, id)
	if err := os.MkdirAll(filepath.Join(dir, "codes"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "checkpoints"), 0700); err != nil {
		return err
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, RoundID: id, StartedAt: now, Status: "active"}
	if err := writeJSON(filepath.Join(dir, "start.json"), state); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return err
	}
	f.round, f.manifest, f.state = dir, manifest, cloneState(state)
	items, err := f.scanCatalogLocked()
	if err == nil {
		_ = f.writeCatalog(items)
	}
	return nil
}
func (f *Filesystem) Append(event game.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(); err != nil {
		return err
	}
	if f.round == "" {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(f.round, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(event); err != nil {
		return err
	}
	state, err := Replay(f.state, []game.Envelope{event}, event.Seq)
	if err != nil {
		return err
	}
	f.state = state
	f.manifest.EventCount++
	if event.Seq > 0 && event.Seq%CheckpointInterval == 0 {
		f.manifest.CheckpointCount++
		return writeJSON(filepath.Join(f.round, "checkpoints", fmt.Sprintf("%020d.json", event.Seq)), Checkpoint{Seq: event.Seq, State: cloneState(f.state)})
	}
	return nil
}
func (f *Filesystem) Code(node string, code game.Code) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(); err != nil {
		return err
	}
	if f.round == "" {
		return errors.New("round archive unavailable")
	}
	name := fmt.Sprintf("%s-%019d-%d.json", safeName(node), code.Version, time.Now().UnixNano())
	return writeJSON(filepath.Join(f.round, "codes", name), code)
}
func (f *Filesystem) StoreBlob(contentType string, data []byte) (BlobRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(); err != nil {
		return BlobRef{}, err
	}
	if f.round == "" {
		return BlobRef{}, errors.New("round archive unavailable")
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	path := filepath.Join(f.round, "blobs", hash)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, data, 0600); err != nil {
			return BlobRef{}, err
		}
		f.manifest.BlobCount++
		f.manifest.BlobBytes += int64(len(data))
	} else if err != nil {
		return BlobRef{}, err
	}
	return BlobRef{RoundID: f.manifest.RoundID, SHA256: hash, Size: int64(len(data)), ContentType: contentType}, nil
}
func (f *Filesystem) DeleteBlob(ref BlobRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if safeName(ref.RoundID) != ref.RoundID || len(ref.SHA256) != 64 {
		return errors.New("invalid blob reference")
	}
	err := os.Remove(filepath.Join(f.root, ref.RoundID, "blobs", ref.SHA256))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil && ref.RoundID == f.manifest.RoundID {
		if f.manifest.BlobCount > 0 {
			f.manifest.BlobCount--
		}
		if f.manifest.BlobBytes >= ref.Size {
			f.manifest.BlobBytes -= ref.Size
		}
	}
	return err
}
func (f *Filesystem) ReadBlob(ref BlobRef) ([]byte, string, error) {
	if safeName(ref.RoundID) != ref.RoundID || len(ref.SHA256) != 64 {
		return nil, "", errors.New("invalid blob reference")
	}
	data, err := os.ReadFile(filepath.Join(f.root, ref.RoundID, "blobs", ref.SHA256))
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != ref.SHA256 || int64(len(data)) != ref.Size {
		return nil, "", errors.New("blob integrity check failed")
	}
	return data, ref.ContentType, nil
}
func (f *Filesystem) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(); err != nil {
		return err
	}
	if f.round == "" {
		return nil
	}
	f.manifest.Status = "complete"
	f.manifest.ClosedAt = time.Now().UTC()
	if err := writeJSON(filepath.Join(f.round, "manifest.json"), f.manifest); err != nil {
		return err
	}
	items, err := f.scanCatalogLocked()
	if err == nil {
		_ = f.writeCatalog(items)
	}
	f.round = ""
	f.manifest = Manifest{}
	return nil
}
func (f *Filesystem) writeCatalog(items []Manifest) error {
	path := filepath.Join(f.root, "catalog.json")
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
func (f *Filesystem) scanCatalogLocked() ([]Manifest, error) {
	entries, err := os.ReadDir(f.root)
	if err != nil {
		return nil, err
	}
	items := []Manifest{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		var manifest Manifest
		data, readErr := os.ReadFile(filepath.Join(f.root, id, "manifest.json"))
		if errors.Is(readErr, os.ErrNotExist) {
			if _, startErr := os.Stat(filepath.Join(f.root, id, "start.json")); startErr == nil {
				items = append(items, Manifest{RoundID: id, Status: "legacy"})
			}
			continue
		}
		if readErr != nil || json.Unmarshal(data, &manifest) != nil || manifest.SchemaVersion != SchemaVersion {
			items = append(items, Manifest{RoundID: id, Status: "corrupt"})
			continue
		}
		items = append(items, manifest)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RoundID > items[j].RoundID })
	return items, nil
}
func (f *Filesystem) List() ([]Manifest, error) {
	var catalog []Manifest
	if err := readJSON(filepath.Join(f.root, "catalog.json"), &catalog); err == nil {
		sort.Slice(catalog, func(i, j int) bool { return catalog[i].RoundID > catalog[j].RoundID })
		return catalog, nil
	}
	entries, err := os.ReadDir(f.root)
	if errors.Is(err, os.ErrNotExist) {
		return []Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		dir := filepath.Join(f.root, id)
		var manifest Manifest
		data, readErr := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if errors.Is(readErr, os.ErrNotExist) {
			if _, startErr := os.Stat(filepath.Join(dir, "start.json")); startErr == nil {
				items = append(items, Manifest{RoundID: id, Status: "legacy"})
			}
			continue
		}
		if readErr != nil || json.Unmarshal(data, &manifest) != nil || manifest.SchemaVersion != SchemaVersion {
			items = append(items, Manifest{RoundID: id, Status: "corrupt"})
			continue
		}
		items = append(items, manifest)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RoundID > items[j].RoundID })
	_ = f.writeCatalog(items)
	return items, nil
}
func (f *Filesystem) Read(id string) (Round, error) {
	if safeName(id) != id {
		return Round{}, errors.New("invalid round id")
	}
	dir := filepath.Join(f.root, id)
	var result Round
	if err := readJSON(filepath.Join(dir, "manifest.json"), &result.Manifest); err != nil {
		return result, err
	}
	if result.Manifest.SchemaVersion != SchemaVersion {
		return result, fmt.Errorf("unsupported archive schema %d", result.Manifest.SchemaVersion)
	}
	if err := readJSON(filepath.Join(dir, "start.json"), &result.Start); err != nil {
		return result, err
	}
	file, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var previous uint64
	checkpointEntries, _ := os.ReadDir(filepath.Join(dir, "checkpoints"))
	for _, entry := range checkpointEntries {
		if entry.IsDir() {
			continue
		}
		var checkpoint Checkpoint
		if err := readJSON(filepath.Join(dir, "checkpoints", entry.Name()), &checkpoint); err == nil {
			result.Checkpoints = append(result.Checkpoints, checkpoint)
		}
	}
	sort.Slice(result.Checkpoints, func(i, j int) bool { return result.Checkpoints[i].Seq < result.Checkpoints[j].Seq })
	for scanner.Scan() {
		var event game.Envelope
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return result, fmt.Errorf("corrupt event: %w", err)
		}
		if previous > 0 && event.Seq <= previous {
			return result, errors.New("non-monotonic event sequence")
		}
		previous = event.Seq
		result.Events = append(result.Events, event)
	}
	return result, scanner.Err()
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func safeName(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return -1
	}, value)
}
