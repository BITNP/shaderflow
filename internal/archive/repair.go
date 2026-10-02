package archive

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"shaderflow/internal/game"
)

type DiagnosticReport struct {
	RoundID       string   `json:"roundId"`
	Healthy       bool     `json:"healthy"`
	Status        string   `json:"status"`
	EventCount    uint64   `json:"eventCount"`
	LastValidSeq  uint64   `json:"lastValidSeq"`
	TruncatedTail bool     `json:"truncatedTail"`
	SequenceError string   `json:"sequenceError,omitempty"`
	BlobErrors    []string `json:"blobErrors,omitempty"`
}
type RepairOptions struct {
	TruncateInvalidTail bool
	CloseRound          bool
}

func (f *Filesystem) Diagnose(id string) (DiagnosticReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.diagnoseLocked(id)
}
func (f *Filesystem) diagnoseLocked(id string) (DiagnosticReport, error) {
	report := DiagnosticReport{RoundID: id, Healthy: true}
	if safeName(id) != id {
		return report, errors.New("invalid round id")
	}
	dir := filepath.Join(f.root, id)
	var manifest Manifest
	if err := readJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return report, err
	}
	report.Status = manifest.Status
	file, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if err == nil {
		reader := bufio.NewReader(file)
		var previous uint64
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				var event game.Envelope
				if decodeErr := json.Unmarshal(line, &event); decodeErr != nil {
					report.Healthy = false
					report.TruncatedTail = readErr == io.EOF
					break
				}
				if previous > 0 && event.Seq != previous+1 {
					report.Healthy = false
					report.SequenceError = fmt.Sprintf("expected seq %d, got %d", previous+1, event.Seq)
					break
				}
				previous = event.Seq
				report.LastValidSeq = event.Seq
				report.EventCount++
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				file.Close()
				return report, readErr
			}
		}
		file.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "blobs"))
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || len(entry.Name()) != 64 {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(dir, "blobs", entry.Name()))
			if readErr != nil || blobHash(data) != entry.Name() {
				report.Healthy = false
				report.BlobErrors = append(report.BlobErrors, entry.Name())
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	return report, nil
}
func (f *Filesystem) Repair(id string, options RepairOptions) (DiagnosticReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	report, err := f.diagnoseLocked(id)
	if err != nil {
		return report, err
	}
	dir := filepath.Join(f.root, id)
	if report.TruncatedTail && options.TruncateInvalidTail {
		path := filepath.Join(dir, "events.jsonl")
		data, err := os.ReadFile(path)
		if err != nil {
			return report, err
		}
		last := bytes.LastIndexByte(data, '\n')
		if last < 0 {
			data = nil
		} else {
			data = data[:last+1]
		}
		if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
			return report, err
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			return report, err
		}
	}
	var manifest Manifest
	if err := readJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return report, err
	}
	manifest.EventCount = report.EventCount
	if options.CloseRound {
		manifest.Status = "complete"
		manifest.ClosedAt = time.Now().UTC()
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return report, err
	}
	items, err := f.scanCatalogLocked()
	if err == nil {
		_ = f.writeCatalog(items)
	}
	return f.diagnoseLocked(id)
}
func blobHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
