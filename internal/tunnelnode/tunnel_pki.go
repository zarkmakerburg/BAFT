package tunnelnode

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// snapshotPKI identifies the entire tree, not just the known certificate
// filenames. Operator-added files and permission changes must also prevent a
// destructive rollback. Symlinks and special files are never followed.
func snapshotPKI(dir string) (map[string]string, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("PKI path is not a real directory")
	}
	digests := map[string]string{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("PKI contains a symlink or special file: %s", path)
		}
		var data []byte
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				file.Close()
				return fmt.Errorf("PKI file changed while checking ownership: %s", path)
			}
			data, err = io.ReadAll(file)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("PKI ownership metadata is unavailable: %s", path)
		}
		metadata := fmt.Sprintf("%o:%d:%d\n", info.Mode(), stat.Uid, stat.Gid)
		digests[rel] = shaHex(append([]byte(metadata), data...))
		return nil
	})
	return digests, err
}

// restorePKI is retryable by identity: after an exchange the old tree is live
// and the installed tree occupies .before-ID. No progress receipt or missing
// directory alone is accepted as proof that restoration already happened.
func (m *Manager) restorePKI(t Txn) error {
	if !t.PKIReplaced {
		return nil
	}
	if len(t.PreviousPKI) == 0 {
		return errors.New("rollback refused: previous PKI identity was not recorded")
	}
	live, err := snapshotPKI(m.pkiDir())
	if err != nil {
		return fmt.Errorf("cannot verify live PKI: %w", err)
	}
	before := m.pkiDir() + ".before-" + t.ID
	backup, err := snapshotPKI(before)
	if err != nil {
		return fmt.Errorf("cannot verify previous PKI backup: %w", err)
	}
	if sameDigests(live, t.PreviousPKI) {
		// The replacement never moved the old tree, or a prior rollback
		// restored it and failed later. Protect an unexpected backup too.
		if len(backup) > 0 && !sameDigests(backup, t.PreviousPKI) && !sameDigests(backup, t.InstalledPKI) {
			return errors.New("rollback refused: PKI backup changed outside BAFT")
		}
		return nil
	}
	if !sameDigests(backup, t.PreviousPKI) {
		return errors.New("rollback refused: previous PKI backup is missing or changed")
	}
	if !sameDigests(live, t.InstalledPKI) && !(len(live) == 0 && len(t.InstalledPKI) == 0) {
		return errors.New("rollback refused: live PKI changed outside BAFT")
	}
	if len(live) == 0 {
		// PKI generation failed before producing a replacement directory.
		err = renameDurable(before, m.pkiDir())
	} else {
		// Linux exchange keeps one complete certificate tree at the live
		// path throughout restoration; preserve the other set until success.
		err = exchangeDirs(m.pkiDir(), before)
	}
	if err != nil {
		return fmt.Errorf("cannot restore previous PKI: %w", err)
	}
	if err := m.fault("tunnel-rollback:after-pki-restore"); err != nil {
		return err
	}
	return nil
}

func (m *Manager) cleanupPKIBackup(t Txn) error {
	if !t.PKIReplaced {
		return nil
	}
	live, err := snapshotPKI(m.pkiDir())
	if err != nil {
		return fmt.Errorf("cannot verify restored PKI before cleanup: %w", err)
	}
	if !sameDigests(live, t.PreviousPKI) {
		return errors.New("rollback refused: restored PKI changed before cleanup")
	}
	before := m.pkiDir() + ".before-" + t.ID
	backup, err := snapshotPKI(before)
	if err != nil {
		return fmt.Errorf("cannot verify PKI rollback cleanup: %w", err)
	}
	if len(backup) == 0 {
		return nil
	}
	if !sameDigests(backup, t.PreviousPKI) && !sameDigests(backup, t.InstalledPKI) {
		return errors.New("rollback refused: PKI backup changed before cleanup")
	}
	return removeAllDurable(before)
}
