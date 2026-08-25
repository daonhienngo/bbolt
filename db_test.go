package bbolt_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func tempDB(t *testing.T) (*bbolt.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	return db, path
}

func TestCleanDatabaseCheck(t *testing.T) {
	db, _ := tempDB(t)
	defer db.Close()

	err := db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		if b == nil {
			t.Fatal("bucket not found")
		}
		return b.Put([]byte("key1"), []byte("value1"))
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	errs := db.Check()
	if len(errs) > 0 {
		t.Fatalf("expected 0 check errors, got: %v", errs)
	}
}

func TestTornMeta0_FallbackToMeta1(t *testing.T) {
	db, path := tempDB(t)
	// Write multiple transactions so meta1 has higher txid
	for i := 0; i < 3; i++ {
		_ = db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("data"))
			return b.Put([]byte("k"), []byte("v"))
		})
	}
	db.Close()

	// Corrupt meta0 (page 0, offset 0 to 4096)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip bytes in meta0 checksum area
	data[32] ^= 0xFF
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Open DB and verify recovery
	reopened, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db with torn meta0: %v", err)
	}
	defer reopened.Close()

	active, tornIdx, warn, err := reopened.ValidateMeta()
	if err != nil {
		t.Fatalf("ValidateMeta failed: %v", err)
	}
	if tornIdx != 0 {
		t.Fatalf("expected torn index 0, got %d", tornIdx)
	}
	if active == nil || active.Txid() == 0 {
		t.Fatalf("expected valid active meta from meta1")
	}
	if warn == "" {
		t.Fatalf("expected diagnostic warning for torn meta0")
	}

	errs := reopened.Check()
	if len(errs) > 0 {
		t.Fatalf("check failed on torn meta0: %v", errs)
	}
}

func TestTornMeta1_FallbackToMeta0(t *testing.T) {
	db, path := tempDB(t)
	for i := 0; i < 2; i++ {
		_ = db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("data"))
			return b.Put([]byte("k"), []byte("v"))
		})
	}
	db.Close()

	// Corrupt meta1 (page 1, offset 4096 to 8192)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[4096+32] ^= 0xFF
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	reopened, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db with torn meta1: %v", err)
	}
	defer reopened.Close()

	active, tornIdx, warn, err := reopened.ValidateMeta()
	if err != nil {
		t.Fatalf("ValidateMeta failed: %v", err)
	}
	if tornIdx != 1 {
		t.Fatalf("expected torn index 1, got %d", tornIdx)
	}
	if active == nil {
		t.Fatalf("expected valid active meta from meta0")
	}
	if warn == "" {
		t.Fatalf("expected diagnostic warning for torn meta1")
	}

	errs := reopened.Check()
	if len(errs) > 0 {
		t.Fatalf("check failed on torn meta1: %v", errs)
	}
}

func TestBothMetaCorrupted_ReturnsFatal(t *testing.T) {
	db, path := tempDB(t)
	db.Close()

	// Corrupt both meta0 and meta1
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[16] ^= 0xFF
	data[4096+16] ^= 0xFF
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err = bbolt.Open(path, 0600, nil)
	if err == nil {
		t.Fatalf("expected fatal error when both meta pages corrupted")
	}
}

func TestCheckIgnoresOrphanPagesBeyondHighWaterMark(t *testing.T) {
	db, path := tempDB(t)
	_ = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		return b.Put([]byte("k"), []byte("v"))
	})
	db.Close()

	// Append uncommitted stray garbage pages to the end of DB file (past meta.pgid)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	garbage := make([]byte, 4096*2)
	for i := range garbage {
		garbage[i] = 0xAA
	}
	if _, err := f.Write(garbage); err != nil {
		t.Fatal(err)
	}
	f.Close()

	reopened, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open db with trailing uncommitted pages: %v", err)
	}
	defer reopened.Close()

	errs := reopened.Check()
	if len(errs) > 0 {
		t.Fatalf("expected Check to ignore uncommitted writes beyond high-water mark, got errors: %v", errs)
	}
}
