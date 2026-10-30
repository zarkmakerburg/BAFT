//go:build unix

package ingressownership

import (
	"bytes"
	"crypto/rand"
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
	dir      string
	identity os.FileInfo
	fault    func(string) error // test-only crash injection
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
	if err := j.withLock(func(d *os.File, _ *os.Root) error {
		info, err := d.Stat()
		j.identity = info
		return err
	}); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *DurableJournal) withLock(run func(*os.File, *os.Root) error) error {
	if j == nil || !filepath.IsAbs(j.dir) || filepath.Clean(j.dir) != j.dir {
		return fmt.Errorf("journal requires a clean absolute directory: %w", ErrInvalid)
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
	if j.identity != nil && !os.SameFile(j.identity, info) {
		return fmt.Errorf("journal directory identity changed since open: %w", ErrInvalid)
	}
	pathInfo, err := os.Lstat(j.dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		return fmt.Errorf("journal directory path differs from locked inode: %w", ErrInvalid)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("journal directory lacks Unix ownership metadata: %w", ErrInvalid)
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || int(stat.Uid) != os.Geteuid() || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("journal directory ownership or mode invalid (uid=%d euid=%d mode=%v): %w", stat.Uid, os.Geteuid(), info.Mode(), ErrInvalid)
	}
	root, err := os.OpenRoot(j.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(info, rootInfo) {
		return fmt.Errorf("journal root differs from locked inode: %w", ErrInvalid)
	}
	if err := run(d, root); err != nil {
		return err
	}
	pathInfo, err = os.Lstat(j.dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		return fmt.Errorf("journal directory moved during operation: %w", ErrInvalid)
	}
	return nil
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

func readJournalFile(root *os.Root, name string, limit int64, result any) error {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
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

func (j *DurableJournal) loadLocked(root *os.Root) (*LabJournal, []journalEntry, error) {
	d, err := root.Open(".")
	if err != nil {
		return nil, nil, err
	}
	names, err := d.ReadDir(-1)
	d.Close()
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
		if err := readJournalFile(root, journalFilename(seq), 4096, &entry); err != nil {
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
	if err := readJournalFile(root, "head.json", 512, &head); err == nil {
		if head.Sequence > uint64(len(entries)) || (head.Sequence != 0 && entries[head.Sequence-1].Digest != head.Digest) || (head.Sequence == 0 && head.Digest != "") {
			return nil, nil, ErrInvalid
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	return journal, entries, nil
}

func (j *DurableJournal) writeAtomic(d *os.File, root *os.Root, name string, raw []byte, replace bool) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	tmpName := ".journal-" + hex.EncodeToString(nonce[:])
	f, err := root.OpenFile(tmpName, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmpName)
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
	if replace {
		var existing journalHead
		if err := readJournalFile(root, name, 512, &existing); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		err = root.Rename(tmpName, name)
	} else {
		err = root.Link(tmpName, name)
		if err == nil {
			err = root.Remove(tmpName)
		}
	}
	if err != nil {
		return err
	}
	return d.Sync()
}

// planJournalMutation decides whether the current phase is an idempotent
// replay before enforcing the append cap. A full journal must still allow a
// crash-retry to repair its durable head without allocating a new entry.
func planJournalMutation(entryCount int, snapshot []MutationRecord, next MutationRecord) (bool, error) {
	for _, current := range snapshot {
		if journalKey(current.Owner) == journalKey(next.Owner) && current == next {
			return true, nil
		}
	}
	if entryCount >= maxJournalEntries {
		return false, ErrInvalid
	}
	return false, nil
}

// Advance persists one validated phase before its corresponding host action.
// Replaying the current record is idempotent, but never skips an earlier step.
func (j *DurableJournal) Advance(next MutationRecord) error {
	return j.withLock(func(d *os.File, root *os.Root) error {
		lab, entries, err := j.loadLocked(root)
		if err != nil {
			return err
		}
		replay, err := planJournalMutation(len(entries), lab.Snapshot(), next)
		if err != nil {
			return err
		}
		if replay {
			if len(entries) == 0 {
				return ErrInvalid
			}
			last := entries[len(entries)-1]
			head, _ := json.Marshal(journalHead{Sequence: last.Sequence, Digest: last.Digest})
			return j.writeAtomic(d, root, "head.json", head, true)
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
		if err := j.writeAtomic(d, root, journalFilename(entry.Sequence), raw, false); err != nil {
			return err
		}
		if j.fault != nil {
			if err := j.fault("after_record"); err != nil {
				return err
			}
		}
		head, _ := json.Marshal(journalHead{Sequence: entry.Sequence, Digest: entry.Digest})
		return j.writeAtomic(d, root, "head.json", head, true)
	})
}

// Recover validates the complete disk chain and returns only a data-only plan.
// No live socket or kernel rule is adopted from journal bytes.
func (j *DurableJournal) Recover() (JournalRecovery, error) {
	var result JournalRecovery
	err := j.withLock(func(_ *os.File, root *os.Root) error {
		lab, _, err := j.loadLocked(root)
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
	err := j.withLock(func(_ *os.File, root *os.Root) error {
		lab, _, err := j.loadLocked(root)
		if err != nil {
			return err
		}
		record, found, err = lab.Current(owner)
		return err
	})
	return record, found, err
}
