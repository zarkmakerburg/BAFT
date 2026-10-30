package bcc

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreNeverReusesPreviouslyIssuedJobID(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNode(Node{ID: "ir", Role: "worker", Address: "127.0.0.1:2222"}, "token-ir"); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateDeployJobs([]string{"ir"}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "before-second-job.baftbak")
	if _, err := app.BackupToFile(backup, backupTestKey(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateDeployJobs([]string{"ir"}, "v1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RestoreFromFile(backup, backupTestKey()); err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateDeployJobs([]string{"ir"}, "v1.2.5")
	if err != nil {
		t.Fatal(err)
	}
	if third[0].ID == first[0].ID || third[0].ID == second[0].ID {
		t.Fatalf("restore reused an issued job ID: %s, %s, %s", first[0].ID, second[0].ID, third[0].ID)
	}
	reopened, err := OpenStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.Jobs[third[0].ID].ID != third[0].ID || reopened.st.NextJob != store.st.NextJob {
		t.Fatal("job sequence did not survive restart")
	}
}
