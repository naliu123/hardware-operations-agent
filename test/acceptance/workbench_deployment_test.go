package acceptance_test

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/sandbox"
)

func TestWB08PostgresDumpAndPrivateFilesRestore(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump is required for deployment backup verification")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("pg_restore is required for deployment backup verification")
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source-files")
	sourceFiles, err := blobstore.Open(sourcePath, 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	sourceDSN, cm := workbenchDatabase(t), wbModel(t)
	sourceServer, _, sourceStop := wbStartOptions(t, sourceDSN, cm, true, application.Options{
		UsersMode: true, Files: sourceFiles, AttachmentRunner: newAttachmentReplayRunner(),
	})
	sourceActive := true
	defer func() {
		if sourceActive {
			sourceStop()
		}
	}()
	source := wbNewClient(t, sourceServer)
	source.login("admin", wbPassword)
	conversation := source.request("POST", "/v1/conversations", map[string]any{}, http.StatusCreated)
	conversationID := conversation["id"].(string)
	content := []byte("2026-10-04T12:00:00Z INFO backup\n2026-10-04T12:01:00Z ERROR restored\n")
	uploaded := uploadAttachment(t, source, conversationID, "backup-restore.log", "text/plain",
		content, http.StatusCreated)
	attachment := waitAttachment(t, source, uploaded["id"].(string))
	if attachment["status"] != "READY" {
		t.Fatalf("source attachment was not durable: %v", attachment)
	}
	attachmentID := attachment["id"].(string)
	expectedHash := sandbox.Hash(content)
	if attachment["sha256"] != expectedHash {
		t.Fatalf("source attachment hash mismatch: %v", attachment)
	}
	sourceStop()
	sourceActive = false

	dump := filepath.Join(root, "database.dump")
	command := exec.Command("pg_dump", "--dbname="+sourceDSN, "--format=custom", "--no-owner", "--file="+dump)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("pg_dump failed: %v: %s", err, output)
	}
	targetDSN := workbenchDatabase(t)
	command = exec.Command("pg_restore", "--dbname="+targetDSN, "--clean", "--if-exists", "--no-owner",
		"--single-transaction", dump)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("pg_restore failed: %v: %s", err, output)
	}
	targetPath := filepath.Join(root, "target-files")
	if err = os.CopyFS(targetPath, os.DirFS(sourcePath)); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(targetPath, 0700); err != nil {
		t.Fatal(err)
	}
	targetFiles, err := blobstore.Open(targetPath, 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	targetServer, _, targetStop := wbStartOptions(t, targetDSN, cm, false, application.Options{
		UsersMode: true, Files: targetFiles, AttachmentRunner: newAttachmentReplayRunner(),
	})
	defer targetStop()
	target := wbNewClient(t, targetServer)
	target.login("admin", wbPassword)
	if restored := target.request("GET", "/v1/conversations/"+conversationID, nil, http.StatusOK); restored["id"] != conversationID {
		t.Fatalf("restored conversation identity changed: %v", restored)
	}
	page := target.request("GET", "/v1/attachments/"+attachmentID+"/pages/1", nil, http.StatusOK)
	if page["attachment"].(map[string]any)["sha256"] != expectedHash ||
		page["page"].(map[string]any)["text"] != string(content) {
		t.Fatalf("restored attachment provenance mismatch: %v", page)
	}
	request, _ := http.NewRequest(http.MethodGet, target.base+"/v1/attachments/"+attachmentID+"/content", nil)
	response, err := target.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	restoredContent, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(restoredContent) != string(content) {
		t.Fatalf("restored private file mismatch: status=%d err=%v body=%q",
			response.StatusCode, readErr, restoredContent)
	}
	t.Log("ACTUAL: PostgreSQL 17 custom dump + private file snapshot restored into a fresh database and file directory; login, conversation, parsed source and original bytes passed")
}
