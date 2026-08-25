package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func TestCLICheckOnCleanDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	code := runCheck(path)
	if code != 0 {
		t.Fatalf("expected exit code 0 for clean DB check, got %d", code)
	}
}

func TestCLICheckOnTornDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		return b.Put([]byte("k"), []byte("v"))
	})
	db.Close()

	// Corrupt meta0
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[20] ^= 0xFF
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	code := runCheck(path)
	if code != 0 {
		t.Fatalf("expected exit code 0 (recovering via meta1) for torn DB check, got %d", code)
	}
}

func TestCLIInfoAndPage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	if code := runInfo(path); code != 0 {
		t.Fatalf("runInfo failed with code %d", code)
	}
	if code := runPage(path, 0); code != 0 {
		t.Fatalf("runPage 0 failed with code %d", code)
	}
	if code := runDump(path, 0); code != 0 {
		t.Fatalf("runDump 0 failed with code %d", code)
	}
}
