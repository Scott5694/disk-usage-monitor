// Package diskusagemonitor periodically measures the total size of a directory
// and invokes a callback when that size crosses a configured threshold.
//
// The design keeps the time source injectable so that tests never depend on
// wall-clock time. The Monitor walks the tree on each Check call; it does not
// cache directory contents because a stale cache is worse than a slightly more
// expensive walk for the intended use case (a background watchdog).
package diskusagemonitor

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Clock abstracts the current time. Tests supply a deterministic fake.
type Clock func() time.Time

// Monitor watches a single directory tree and reports when its total size
// exceeds Threshold bytes. It is not safe for concurrent use; callers that
// need concurrency should serialise Check calls themselves.
type Monitor struct {
	// Dir is the root of the tree being measured.
	Dir string
	// Threshold is the size in bytes at or above which OnExceed is called.
	Threshold int64
	// Interval is how often Run should call Check.
	Interval time.Duration
	// OnExceed is invoked with the measured size whenever it is >= Threshold.
	OnExceed func(size int64)

	clock Clock
}

// New returns a Monitor rooted at dir. The clock argument is required so that
// callers (and tests) control time explicitly; passing nil uses time.Now.
func New(dir string, threshold int64, interval time.Duration, onExceed func(int64), clock Clock) *Monitor {
	if clock == nil {
		clock = time.Now
	}
	return &Monitor{
		Dir:       dir,
		Threshold: threshold,
		Interval:  interval,
		OnExceed:  onExceed,
		clock:     clock,
	}
}

// Check walks the tree once and reports the total size of regular files.
// If the total is >= Threshold and OnExceed is non-nil, OnExceed is called
// with the total. The measured size is always returned, even when the
// callback is skipped.
//
// Symlinks are not followed: a symlink loop would otherwise make the walk
// unbounded. Directories themselves contribute zero bytes.
func (m *Monitor) Check() (int64, error) {
	total, err := DirSize(m.Dir)
	if err != nil {
		return 0, err
	}
	if m.OnExceed != nil && total >= m.Threshold {
		m.OnExceed(total)
	}
	return total, nil
}

// Run calls Check every Interval until stop is closed. It returns the last
// error seen, or nil if stop was closed cleanly. A non-positive Interval is
// treated as an error-free no-op that still honours stop.
func (m *Monitor) Run(stop <-chan struct{}) error {
	if m.Interval <= 0 {
		<-stop
		return nil
	}
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-stop:
			return lastErr
		case <-ticker.C:
			if _, err := m.Check(); err != nil {
				lastErr = err
			}
		}
	}
}

// DirSize returns the sum of the sizes of all regular files under root.
// It is exported because callers often want a one-shot measurement without
// constructing a Monitor.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			// Do not follow symlinks; see Monitor.Check.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// ensure os is used even if future refactors drop the explicit reference;
// os.Stat is the canonical way to obtain FileInfo for a path.
var _ = os.Stat
