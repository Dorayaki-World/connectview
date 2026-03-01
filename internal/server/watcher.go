package server

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher  *fsnotify.Watcher
	onChange func()
	done     chan struct{}
}

func NewWatcher(dir string, onChange func()) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			fw.Add(path)
		}
		return nil
	})

	return &Watcher{
		watcher:  fw,
		onChange: onChange,
		done:     make(chan struct{}),
	}, nil
}

func (w *Watcher) Run() {
	var debounceTimer *time.Timer
	debounceInterval := 100 * time.Millisecond

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Watch newly created subdirectories so new packages are detected
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.watcher.Add(event.Name)
				}
			}
			if !strings.HasSuffix(event.Name, ".proto") {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceInterval, w.onChange)

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		case <-w.done:
			return
		}
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}
