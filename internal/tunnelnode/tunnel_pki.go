package tunnelnode

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func (m *Manager) preparedPKIDir(t Txn) string {
	if t.PKIStaged {
		return m.pkiNext(t.ID)
	}
	return m.pkiDir()
}

func (m *Manager) validatePreparedCertificate(dir, host string) error {
	b, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		return err
	}
	cas, err := parseCerts(b)
	if err != nil || len(cas) != 1 || !cas[0].IsCA || cas[0].CheckSignatureFrom(cas[0]) != nil {
		return errors.New("prepared PKI must contain one self-signed CA certificate")
	}
	leaf, err := readLeaf(filepath.Join(dir, "server.pem"))
	if err != nil {
		return err
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: poolOf(cas), DNSName: host, CurrentTime: m.Now()}); err != nil {
		return fmt.Errorf("prepared certificate does not verify: %w", err)
	}
	for _, pair := range [][2]string{{"server.pem", "server.key"}, {"ca.pem", "ca.key"}} {
		if _, err := tls.LoadX509KeyPair(filepath.Join(dir, pair[0]), filepath.Join(dir, pair[1])); err != nil {
			return fmt.Errorf("prepared certificate/key pair does not match: %w", err)
		}
	}
	return nil
}

// Unlike sameDigests, this also compares the deliberate absence of a tree.
func equalPKI(a, b map[string]string) bool {
	return len(a) == 0 && len(b) == 0 || sameDigests(a, b)
}

func (m *Manager) verifyPreparedPKI(t Txn) error {
	if t.PKIHost == "" { // existing in-flight transaction contract
		return nil
	}
	live, err := snapshotPKI(m.pkiDir())
	if err != nil || !equalPKI(live, t.PreviousPKI) {
		return errors.New("commit refused: live PKI changed after preparation")
	}
	candidate, err := snapshotPKI(m.preparedPKIDir(t))
	if err != nil || !sameDigests(candidate, t.InstalledPKI) {
		return errors.New("commit refused: prepared PKI is missing or changed")
	}
	return m.validatePreparedCertificate(m.preparedPKIDir(t), t.PKIHost)
}

func (m *Manager) activatePreparedPKI(t Txn) error {
	if t.PKIHost == "" {
		return nil
	}
	if err := m.verifyPreparedPKI(t); err != nil {
		return err
	}
	if !t.PKIStaged {
		return nil
	}
	before, next := m.pkiDir()+".before-"+t.ID, m.pkiNext(t.ID)
	if _, err := os.Lstat(before); !errors.Is(err, os.ErrNotExist) {
		return errors.New("commit refused: PKI backup path is occupied or unreadable")
	}
	if len(t.PreviousPKI) > 0 {
		if err := exchangeDirs(m.pkiDir(), next); err != nil {
			return err
		}
		if err := m.fault("tunnel-commit:after-pki-exchange"); err != nil {
			return err
		}
		if err := renameDurable(next, before); err != nil {
			return err
		}
	} else if err := renameDurable(next, m.pkiDir()); err != nil {
		return err
	}
	return m.fault("tunnel-commit:after-pki-activation")
}

func (m *Manager) verifyPKIFinalization(t Txn) error {
	if t.PKIHost == "" {
		return nil
	}
	live, err := snapshotPKI(m.pkiDir())
	if err != nil || !sameDigests(live, t.InstalledPKI) {
		return errors.New("finalize refused: installed PKI changed; backups retained")
	}
	next, err := snapshotPKI(m.pkiNext(t.ID))
	if err != nil || len(next) > 0 {
		return errors.New("finalize refused: candidate path is unexpectedly occupied")
	}
	backup, err := snapshotPKI(m.pkiDir() + ".before-" + t.ID)
	if err != nil {
		return err
	}
	if t.PKIReplaced {
		if !sameDigests(backup, t.PreviousPKI) && !(t.Phase == PhaseFinalizing && len(backup) == 0) {
			return errors.New("finalize refused: previous PKI backup is missing or changed")
		}
	} else if len(backup) > 0 {
		return errors.New("finalize refused: unexpected PKI backup; retained")
	}
	return nil
}

// The old tree can be at either next or before if commit stopped between the
// atomic exchange and the durable rename. Recorded identities decide which
// tree can move; directory existence alone never grants deletion authority.
func (m *Manager) restoreStagedPKI(t Txn) error {
	live, err := snapshotPKI(m.pkiDir())
	if err != nil {
		return err
	}
	before, next := m.pkiDir()+".before-"+t.ID, m.pkiNext(t.ID)
	oldPath := ""
	for _, path := range []string{before, next} {
		identity, err := snapshotPKI(path)
		if err != nil {
			return err
		}
		if len(identity) > 0 && !sameDigests(identity, t.PreviousPKI) && !sameDigests(identity, t.InstalledPKI) {
			return errors.New("rollback refused: prepared/backup PKI changed outside BAFT")
		}
		if sameDigests(identity, t.PreviousPKI) {
			oldPath = path
		}
	}
	if equalPKI(live, t.PreviousPKI) {
		return nil // prepared cancellation or an explicitly retried rollback
	}
	if !t.PKIStaged || !sameDigests(live, t.InstalledPKI) {
		return errors.New("rollback refused: live PKI changed outside BAFT")
	}
	if len(t.PreviousPKI) > 0 {
		if oldPath == "" {
			return errors.New("rollback refused: previous PKI backup is missing or changed")
		}
		if err := exchangeDirs(m.pkiDir(), oldPath); err != nil {
			return err
		}
	} else {
		if _, err := os.Lstat(next); !errors.Is(err, os.ErrNotExist) {
			return errors.New("rollback refused: candidate path is occupied")
		}
		if err := renameDurable(m.pkiDir(), next); err != nil {
			return err
		}
	}
	return m.fault("tunnel-rollback:after-pki-restore")
}

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
	if t.PKIHost != "" {
		return m.restoreStagedPKI(t)
	}
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
	if t.PKIHost != "" {
		live, err := snapshotPKI(m.pkiDir())
		if err != nil || !equalPKI(live, t.PreviousPKI) {
			return errors.New("rollback refused: restored PKI changed before cleanup")
		}
		for _, path := range []string{m.pkiNext(t.ID), m.pkiDir() + ".before-" + t.ID} {
			identity, err := snapshotPKI(path)
			if err != nil {
				return err
			}
			if len(identity) == 0 {
				continue
			}
			if !sameDigests(identity, t.InstalledPKI) && !sameDigests(identity, t.PreviousPKI) {
				return errors.New("rollback refused: prepared/backup PKI changed before cleanup")
			}
			if err := removeAllDurable(path); err != nil {
				return err
			}
		}
		return nil
	}
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
