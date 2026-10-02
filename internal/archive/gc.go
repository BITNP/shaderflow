package archive

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"shaderflow/internal/game"
	"strings"
	"time"
)

type GCOptions struct {
	MinimumAge time.Duration
	DryRun     bool
}
type GCReport struct {
	ScannedRounds int   `json:"scannedRounds"`
	ScannedBlobs  int   `json:"scannedBlobs"`
	OrphanCount   int   `json:"orphanCount"`
	OrphanBytes   int64 `json:"orphanBytes"`
	DeletedCount  int   `json:"deletedCount"`
	DeletedBytes  int64 `json:"deletedBytes"`
	SkippedActive int   `json:"skippedActive"`
}

func (f *Filesystem) GarbageCollectBlobs(options GCOptions) (GCReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var report GCReport
	items, err := f.scanCatalogLocked()
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	now := time.Now()
	for index := range items {
		item := &items[index]
		if item.Status == "active" {
			report.SkippedActive++
			continue
		}
		if item.Status != "complete" {
			continue
		}
		report.ScannedRounds++
		round, err := f.readUnlocked(item.RoundID)
		if err != nil {
			return report, err
		}
		references := map[string]bool{}
		for _, input := range round.Start.ExternalInputs {
			if len(input.SHA256) == 64 {
				references[input.SHA256] = true
			}
		}
		for _, snapshot := range round.Start.RenderSnapshots {
			if len(snapshot.SHA256) == 64 {
				references[snapshot.SHA256] = true
			}
		}
		for _, event := range round.Events {
			if event.Event == nil {
				continue
			}
			switch event.Event.Type {
			case "input.updated":
				var input struct {
					SHA256 string `json:"sha256"`
				}
				if convert(event.Event.Data, &input) == nil && len(input.SHA256) == 64 {
					references[input.SHA256] = true
				}
			case "snapshot.created":
				var snapshot struct {
					SHA256 string `json:"sha256"`
				}
				if convert(event.Event.Data, &snapshot) == nil && len(snapshot.SHA256) == 64 {
					references[snapshot.SHA256] = true
				}
			}
		}
		entries, err := os.ReadDir(filepath.Join(f.root, item.RoundID, "blobs"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return report, err
		}
		var keptCount uint64
		var keptBytes int64
		for _, entry := range entries {
			if entry.IsDir() || len(entry.Name()) != 64 || strings.ContainsAny(entry.Name(), "/\\") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return report, err
			}
			report.ScannedBlobs++
			if references[entry.Name()] {
				keptCount++
				keptBytes += info.Size()
				continue
			}
			if now.Sub(info.ModTime()) < options.MinimumAge {
				keptCount++
				keptBytes += info.Size()
				continue
			}
			report.OrphanCount++
			report.OrphanBytes += info.Size()
			if !options.DryRun {
				if err := os.Remove(filepath.Join(f.root, item.RoundID, "blobs", entry.Name())); err != nil {
					return report, err
				}
				report.DeletedCount++
				report.DeletedBytes += info.Size()
			} else {
				keptCount++
				keptBytes += info.Size()
			}
		}
		if !options.DryRun {
			item.BlobCount = keptCount
			item.BlobBytes = keptBytes
			if item.RoundID == f.manifest.RoundID {
				f.manifest.BlobCount = keptCount
				f.manifest.BlobBytes = keptBytes
			}
		}
	}
	if !options.DryRun {
		if err := f.writeCatalog(items); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (f *Filesystem) readUnlocked(id string) (Round, error) {
	if safeName(id) != id {
		return Round{}, errors.New("invalid round id")
	}
	dir := filepath.Join(f.root, id)
	var result Round
	if err := readJSON(filepath.Join(dir, "manifest.json"), &result.Manifest); err != nil {
		return result, err
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
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var event game.Envelope
		if err := decoder.Decode(&event); err != nil {
			return result, err
		}
		result.Events = append(result.Events, event)
	}
	return result, nil
}
