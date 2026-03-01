package server_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dorayaki-World/connectview/internal/server"
)

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	protoFile := filepath.Join(dir, "test.proto")
	os.WriteFile(protoFile, []byte(`syntax = "proto3";`), 0644)

	var called atomic.Int32
	w, err := server.NewWatcher(dir, func() {
		called.Add(1)
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	go w.Run()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	os.WriteFile(protoFile, []byte(`syntax = "proto3"; // modified`), 0644)

	// Wait for debounce + callback
	time.Sleep(300 * time.Millisecond)

	if called.Load() == 0 {
		t.Error("expected onChange callback to be called")
	}
}

func TestWatcher_DebouncesBurstChanges(t *testing.T) {
	dir := t.TempDir()
	protoFile := filepath.Join(dir, "test.proto")
	os.WriteFile(protoFile, []byte(`syntax = "proto3";`), 0644)

	var callCount atomic.Int32
	w, err := server.NewWatcher(dir, func() {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	go w.Run()
	time.Sleep(50 * time.Millisecond)

	// Rapid-fire changes
	for i := 0; i < 5; i++ {
		os.WriteFile(protoFile, []byte(fmt.Sprintf("// change %d", i)), 0644)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)

	count := callCount.Load()
	if count > 2 {
		t.Errorf("expected debounced calls (<=2), got %d", count)
	}
}
