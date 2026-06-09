package server

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStoreProtocol(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "ioa.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runStoreProtocolTest(t, store)
}

func TestSQLiteStoreContentSchema(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "ioa.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runContentSchemaTest(t, store)
}

func TestSQLiteStoreProjections(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "ioa.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runProjectionTest(t, store)
}
