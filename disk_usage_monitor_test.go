package diskusagemonitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile is a tiny helper that fails the test on any I/O error.
func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestDirSizeSumsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 10)
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), 25)
	writeFile(t, filepath.Join(dir, "sub", "deep", "c.txt"), 5)

	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize: %v", err)
	}
	want := int64(40)
	if got != want {
		t.Fatalf("DirSize = %d, want %d", got, want)
	}
}

func TestDirSizeEmptyDir(t *testing.T) {
	dir := t.TempDir()
	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize: %v", err)
	}
	if got != 0 {
		t.Fatalf("DirSize = %d, want 0", got)
	}
}

func TestDirSizeMissingRoot(t *testing.T) {
	_, err := DirSize(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestDirSizeDoesNotFollowSymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	writeFile(t, real, 7)
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize: %v", err)
	}
	// Only the real file counts; the symlink is skipped.
	if got != 7 {
		t.Fatalf("DirSize = %d, want 7", got)
	}
}

func TestCheckTriggersCallbackAtThreshold(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 100)

	var called int64
	var calls int
	m := New(dir, 100, time.Second, func(size int64) {
		calls++
		called = size
	}, nil)

	size, err := m.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if size != 100 {
		t.Fatalf("size = %d, want 100", size)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
	if called != 100 {
		t.Fatalf("callback size = %d, want 100", called)
	}
}

func TestCheckDoesNotTriggerBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 99)

	var calls int
	m := New(dir, 100, time.Second, func(int64) { calls++ }, nil)

	size, err := m.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if size != 99 {
		t.Fatalf("size = %d, want 99", size)
	}
	if calls != 0 {
		t.Fatalf("callback calls = %d, want 0", calls)
	}
}

func TestCheckNilCallbackIsSafe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 10)
	m := New(dir, 1, time.Second, nil, nil)

	size, err := m.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if size != 10 {
		t.Fatalf("size = %d, want 10", size)
	}
}

func TestRunHonoursStopWithNonPositiveInterval(t *testing.T) {
	dir := t.TempDir()
	m := New(dir, 1, 0, func(int64) {}, nil)

	stop := make(chan struct{})
	close(stop)

	done := make(chan error, 1)
	go func() { done <- m.Run(stop) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after stop closed")
	}
}

func TestRunTicksAndStops(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), 5)

	var calls int
	m := New(dir, 1, time.Millisecond, func(int64) { calls++ }, nil)

	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- m.Run(stop) }()

	// Wait for at least one tick to fire.
	deadline := time.After(2 * time.Second)
	for calls == 0 {
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatal("no tick observed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
}

func TestNewNilClockFallsBackToTimeNow(t *testing.T) {
	m := New(t.TempDir(), 0, time.Second, nil, nil)
	if m.clock == nil {
		t.Fatal("clock is nil")
	}
	if m.clock().IsZero() {
		t.Fatal("clock returned zero time")
	}
}
