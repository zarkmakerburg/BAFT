//go:build unix

package ingressownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// DurableJournal records an intent before the caller changes any resource.
// Its records are evidence of intent and progress, never evidence that a
// listener or kernel rule still belongs to BAFT after restart.
type DurableJournal struct {
	dir   string
	fault func(string) error // test-only crash injection
}

type journalEntry struct {
	Version  int            `json:"version"`
	Sequence uint64         `json:"sequence"`
	Previous string         `json:"previous"`
	Mutation MutationRecord `json:"mutation"`
	Digest   string         `json:"digest"`
}

type journalHead struct {
	Sequence uint64 `json:"sequence"`
	Digest   string `json:"digest"`
}

const maxJournalEntries = 10000

func OpenDurableJournal(dir string) (*DurableJournal, error) {
	j := &DurableJournal{dir: dir}
	if err := j.withLock(func(*os.File) error { return nil }); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *DurableJournal) withLock(run func(*os.File) error) error {
	if j == nil || !filepath.IsAbs(j.dir) || filepath.Clean(j.dir) != j.dir {
		return ErrInvalid
	}
	d, err := os.Open(j.dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(d.Fd()), syscall.LOCK_UN)
	info, err := d.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(j.dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		return ErrInvalid
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || int(stat.Uid) != os.Geteuid() || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrInvalid
	}
	return run(d)
}

func journalFilename(sequence uint64) string { return fmt.Sprintf("%020d.json", sequence) }

func entryDigest(entry journalEntry) (string, error) {
	entry.Digest = ""
	raw, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func readJournalFile(path string, limit int64, result any) error {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || int(stat.Uid) != os.Geteuid() || stat.Nlink != 1 || info.Size() == 0 || info.Size() > limit {
		return ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit || json.Unmarshal(raw, result) != nil {
		return ErrInvalid
	}
	canonical, err := json.Marshal(result)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrInvalid
	}
	return nil
}

func (j *DurableJournal) loadLocked() (*LabJournal, []journalEntry, error) {
	names, err := os.ReadDir(j.dir)
	if err != nil {
		return nil, nil, err
	}
	sequences := make([]uint64, 0, len(names))
	for _, n := range names {
		name := n.Name()
		if strings.HasPrefix(name, ".journal-") || name == "head.json" {
			continue
		}
		if len(name) != len(journalFilename(1)) || !strings.HasSuffix(name, ".json") {
			return nil, nil, ErrInvalid
		}
		seq, err := strconv.ParseUint(strings.TrimSuffix(name, ".json"), 10, 64)
		if err != nil || seq == 0 || journalFilename(seq) != name {
			return nil, nil, ErrInvalid
		}
		sequences = append(sequences, seq)
	}
	if len(sequences) > maxJournalEntries {
		return nil, nil, ErrInvalid
	}
	sort.Slice(sequences, func(a, b int) bool { return sequences[a] < sequences[b] })
	journal := new(LabJournal)
	entries := make([]journalEntry, 0, len(sequences))
	previous := ""
	for i, seq := range sequences {
		if seq != uint64(i+1) {
			return nil, nil, ErrInvalid
		}
		var entry journalEntry
		if err := readJournalFile(filepath.Join(j.dir, journalFilename(seq)), 4096, &entry); err != nil {
			return nil, nil, err
		}
		digest, err := entryDigest(entry)
		if err != nil || entry.Version != 1 || entry.Sequence != seq || entry.Previous != previous || entry.Digest != digest {
			return nil, nil, ErrInvalid
		}
		if err := journal.Advance(entry.Mutation); err != nil {
			return nil, nil, err
		}
		previous = digest
		entries = append(entries, entry)
	}
	var head journalHead
	if err := readJournalFile(filepath.Join(j.dir, "head.json"), 512, &head); err == nil {
		if head.Sequence > uint64(len(entries)) || (head.Sequence != 0 && entries[head.Sequence-1].Digest != head.Digest) || (head.Sequence == 0 && head.Digest != "") {
			return nil, nil, ErrInvalid
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	return journal, entries, nil
}

func (j *DurableJournal) writeAtomic(d *os.File, name string, raw []byte, replace bool) error {
	f, err := os.CreateTemp(j.dir, ".journal-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	path := filepath.Join(j.dir, name)
	if replace {
		var existing journalHead
		if err := readJournalFile(path, 512, &existing); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		err = os.Rename(f.Name(), path)
	} else {
		err = os.Link(f.Name(), path)
		if err == nil {
			err = os.Remove(f.Name())
		}
	}
	if err != nil {
		return err
	}
	return d.Sync()
}

// Advance persists one validated phase before its corresponding host action.
// Replaying the current record is idempotent, but never skips an earlier step.
func (j *DurableJournal) Advance(next MutationRecord) error {
	return j.withLock(func(d *os.File) error {
		lab, entries, err := j.loadLocked()
		if err != nil {
			return err
		}
		if len(entries) == maxJournalEntries {
			return ErrInvalid
		}
		for _, current := range lab.Snapshot() {
			if journalKey(current.Owner) == journalKey(next.Owner) && current == next {
				if len(entries) == 0 {
					return ErrInvalid
				}
				last := entries[len(entries)-1]
				head, _ := json.Marshal(journalHead{Sequence: last.Sequence, Digest: last.Digest})
				return j.writeAtomic(d, "head.json", head, true)
			}
		}
		if err := lab.Advance(next); err != nil {
			return err
		}
		entry := journalEntry{Version: 1, Sequence: uint64(len(entries) + 1), Mutation: next}
		if len(entries) != 0 {
			entry.Previous = entries[len(entries)-1].Digest
		}
		entry.Digest, err = entryDigest(entry)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if j.fault != nil {
			if err := j.fault("before_record"); err != nil {
				return err
			}
		}
		if err := j.writeAtomic(d, journalFilename(entry.Sequence), raw, false); err != nil {
			return err
		}
		if j.fault != nil {
			if err := j.fault("after_record"); err != nil {
				return err
			}
		}
		head, _ := json.Marshal(journalHead{Sequence: entry.Sequence, Digest: entry.Digest})
		return j.writeAtomic(d, "head.json", head, true)
	})
}

// Recover validates the complete disk chain and returns only a data-only plan.
// No live socket or kernel rule is adopted from journal bytes.
func (j *DurableJournal) Recover() (JournalRecovery, error) {
	var result JournalRecovery
	err := j.withLock(func(_ *os.File) error {
		lab, _, err := j.loadLocked()
		if err != nil {
			return err
		}
		_, result, err = RecoverLabJournal(lab.Snapshot())
		return err
	})
	return result, err
}

func (j *DurableJournal) Current(owner ingressmanifest.Proof) (MutationRecord, bool, error) {
	var record MutationRecord
	var found bool
	err := j.withLock(func(_ *os.File) error {
		lab, _, err := j.loadLocked()
		if err != nil {
			return err
		}
		record, found, err = lab.Current(owner)
		return err
	})
	return record, found, err
}
