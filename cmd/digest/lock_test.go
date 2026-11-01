package main

import (
	"path/filepath"
	"testing"
)

func TestLockExclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "digest.lock")
	release, err := acquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireLock(p); err == nil {
		t.Fatal("second acquire should fail while held")
	}
	release()
	release2, err := acquireLock(p)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release2()
}
