# disk-usage-monitor

Periodically measures the total size of a directory tree and calls a callback when the total reaches a configured byte threshold.

```go
package main

import (
	"fmt"
	"time"

	diskusagemonitor "example/disk-usage-monitor"
)

func main() {
	m := diskusagemonitor.New(
		"/var/log",
		1<<30, // 1 GiB
		time.Minute,
		func(size int64) {
			fmt.Printf("log directory is now %d bytes\n", size)
		},
		nil, // use time.Now
	)

	stop := make(chan struct{})
	defer close(stop)
	if err := m.Run(stop); err != nil {
		fmt.Println("monitor stopped:", err)
	}
}
```

For a one-shot measurement without a Monitor:

```go
size, err := diskusagemonitor.DirSize("/var/log")
```

## Why this exists

A disk-usage watchdog usually only needs two things: a byte total and a signal when that total crosses a line. Pulling in a full metrics framework for that is overkill, and the frameworks tend to hide the time source, which makes the watchdog impossible to test without sleeping. This library is deliberately small: one walk, one comparison, one callback. The trade-off is that it re-walks the tree on every check instead of caching directory contents. A cache would make repeated checks cheaper but would also report stale sizes, which defeats the purpose of a watchdog.

## The awkward edge

The walk does not follow symlinks. A symlink loop would otherwise make the traversal unbounded, and a symlinked file would be counted twice (once through its real path and once through the link). If your directory contains symlinks to files that you do want counted, resolve them before passing the root to this library, or measure the resolved directories separately.

## API

- `New(dir string, threshold int64, interval time.Duration, onExceed func(int64), clock Clock) *Monitor`
- `(*Monitor) Check() (int64, error)`
- `(*Monitor) Run(stop <-chan struct{}) error`
- `DirSize(root string) (int64, error)`
- `Clock` is `func() time.Time`; pass `nil` to `New` to use `time.Now`.

The callback fires when the measured size is greater than or equal to the threshold.
