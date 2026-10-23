package bcc

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Re-authenticate a test fixture under the legacy schema2 header, as a real
// previous binary would have written it. Never merely edit authenticated AAD.
func legacyReservationBackupFixture(t *testing.T, raw, key []byte, version int) []byte {
	t.Helper()
	var env encryptedBackup
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	aead, err := newAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	aad, _ := json.Marshal(env.Header)
	nonce, _ := base64.StdEncoding.DecodeString(env.Nonce)
	ciphertext, _ := base64.StdEncoding.DecodeString(env.Ciphertext)
	plain, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	env.Header.SchemaVersion = version
	aad, _ = json.Marshal(env.Header)
	nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	env.Nonce = base64.StdEncoding.EncodeToString(nonce)
	env.Ciphertext = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, plain, aad))
	result, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublicIngressReservationBackupVersionCompatibility(t *testing.T) {
	s, spec := reservationStore(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw, header, err := app.CreateBackupBytes(backupTestKey(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if header.SchemaVersion != BackupSchemaVersion {
		t.Fatalf("new backup schema %d allows older binaries to silently drop reservation state", header.SchemaVersion)
	}
	old := legacyReservationBackupFixture(t, raw, backupTestKey(), 2)
	payload, oldHeader, err := decodeBackup(old, backupTestKey())
	if err != nil || oldHeader.SchemaVersion != 2 || len(payload.State.PublicIngressReservations) != 0 {
		t.Fatalf("valid previous backup refused: %+v %v", oldHeader, err)
	}
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	if err := app.FlushSecurityAuditIntents(); err != nil {
		t.Fatal(err)
	}
	raw, header, err = app.CreateBackupBytes(backupTestKey(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if header.SchemaVersion != BackupSchemaVersion {
		t.Fatal("reservation backup downgrade permitted")
	}
	payload, _, err = decodeBackup(raw, backupTestKey())
	if err != nil || len(payload.State.PublicIngressReservations) != 1 {
		t.Fatalf("new reservation backup failed: %v", err)
	}
	if _, _, err := decodeBackup(legacyReservationBackupFixture(t, raw, backupTestKey(), 2), backupTestKey()); err == nil || !strings.Contains(err.Error(), "legacy backup schema") {
		t.Fatalf("legacy header accepted reservation-bearing payload: %v", err)
	}
}

func TestPublicIngressOperationBackupVersionCompatibility(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil { t.Fatal(err) }
	app, err := NewServer(s, "admin")
	if err != nil { t.Fatal(err) }
	if err := app.FlushSecurityAuditIntents(); err != nil { t.Fatal(err) }
	raw, _, err := app.CreateBackupBytes(backupTestKey(), time.Now())
	if err != nil { t.Fatal(err) }
	if _, _, err := decodeBackup(legacyReservationBackupFixture(t, raw, backupTestKey(), 3), backupTestKey()); err != nil { t.Fatalf("schema 3 reservation backup refused: %v", err) }
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil { t.Fatal(err) }
	if err := app.FlushSecurityAuditIntents(); err != nil { t.Fatal(err) }
	raw, _, err = app.CreateBackupBytes(backupTestKey(), time.Now())
	if err != nil { t.Fatal(err) }
	if _, _, err := decodeBackup(legacyReservationBackupFixture(t, raw, backupTestKey(), 3), backupTestKey()); err == nil || !strings.Contains(err.Error(), "legacy backup schema") { t.Fatalf("schema 3 accepted operation-bearing backup: %v", err) }
}
