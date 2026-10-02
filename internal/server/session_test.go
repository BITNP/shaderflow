package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusivePlayerSessionRevokesPreviousDeviceAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	store, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.createExclusive("u", "User", "player")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.createExclusive("u", "User", "player")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.get(first); ok {
		t.Fatal("previous player device remained authenticated")
	}
	if _, ok := store.get(second); !ok {
		t.Fatal("new player device rejected")
	}
	restarted, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := restarted.get(first); ok {
		t.Fatal("revoked device restored after restart")
	}
	if _, ok := restarted.get(second); !ok {
		t.Fatal("exclusive session not persisted")
	}
}

func TestFileSessionStoreSurvivesRestartAndRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	first, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := first.create("u", "User", "player")
	if err != nil {
		t.Fatal(err)
	}
	second, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := second.get(id)
	if !ok || got.Sub != "u" {
		t.Fatalf("session not restored: %#v %v", got, ok)
	}
	second.delete(id)
	third, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := third.get(id); ok {
		t.Fatal("revoked session restored")
	}
}

func TestFileSessionStoreDropsExpiredAndRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	store, err := newFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	store.ttl = -time.Second
	id, err := store.create("u", "User", "player")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.get(id); ok {
		t.Fatal("expired session accepted")
	}
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileSessionStore(path); err == nil {
		t.Fatal("corrupt session file accepted")
	}
}
