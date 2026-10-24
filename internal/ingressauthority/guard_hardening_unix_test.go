//go:build unix

package ingressauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
)

func TestPendingGuardRefusesAliasReplacementAndPostClose(t *testing.T) {
	dir := privateReservationDir(t)
	store, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.CheckPending(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	replacement := dir + "-old"
	if err := os.Rename(dir, replacement); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckPending(context.Background(), b); !errors.Is(err, ErrUnverified) {
		t.Fatalf("replaced inode accepted: %v", err)
	}
	if _, err := store.HoldPending(b); !errors.Is(err, ErrUnverified) {
		t.Fatalf("new path accepted: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckPending(context.Background(), b); !errors.Is(err, ErrUnverified) {
		t.Fatalf("closed lock accepted: %v", err)
	}
	if err := g.Close(); !errors.Is(err, ErrUnverified) {
		t.Fatalf("double close accepted: %v", err)
	}
}

func TestAuthorityDirectoryRefusesSymlinksAndFIFO(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "original")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(original, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(parent, "pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, filepath.Join(parent, "regular")} {
		if path == filepath.Join(parent, "regular") {
			if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := OpenReservationStore(path); !errors.Is(err, ErrUnverified) {
			t.Fatalf("unsafe directory %q admitted: %v", path, err)
		}
	}
}

func TestPendingGuardConcurrentChecksAndClose(t *testing.T) {
	dir := privateReservationDir(t)
	store, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.CheckPending(context.Background(), b)
			if err != nil && !errors.Is(err, ErrUnverified) {
				t.Errorf("check: %v", err)
			}
		}()
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := g.CheckPending(context.Background(), b); !errors.Is(err, ErrUnverified) {
		t.Fatalf("post-close: %v", err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatalf("durable claim lost: %v", err)
	}
}

func TestAuthorityRootStatMustMatchLockedInode(t *testing.T) {
	first := privateReservationDir(t)
	second := privateReservationDir(t)
	fd, err := openPinnedAuthorityDir(first, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	foreign, err := os.OpenRoot(second)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if err := checkAuthorityRoot(fd, foreign, first, nil); !errors.Is(err, ErrUnverified) {
		t.Fatalf("mismatched root.Stat(.) accepted: %v", err)
	}
}

func TestAuthorityDirectoryRejectsSpecialBitsAndUnsafePermissions(t *testing.T) {
	dir := privateReservationDir(t)
	for _, mode := range []os.FileMode{0770, 0777, 0700 | os.ModeSetgid, 0700 | os.ModeSticky} {
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() == 0700 {
			continue
		} // unsupported special bit on this FS
		if _, err := openPinnedAuthorityDir(dir, nil); !errors.Is(err, ErrUnverified) {
			t.Fatalf("unsafe mode %v accepted: %v", info.Mode(), err)
		}
	}
}
