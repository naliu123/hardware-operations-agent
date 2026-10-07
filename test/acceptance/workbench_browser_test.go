package acceptance_test

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/identity"
	"hwops/internal/knowledge"
	"hwops/internal/transport/httpapi"
)

// Opt-in browser fixture: real temporary PostgreSQL, same-origin Go hosting,
// external deterministic model adapter. SIGTERM ends it and drops the test DB.
// The synthetic account is admin / wb01-synthetic-password, never a deployment.
func TestWB01BrowserFixture(t *testing.T) {
	if os.Getenv("HWOPS_BROWSER_TEST") != "1" {
		t.Skip("interactive browser fixture disabled")
	}
	dsn, cm := workbenchDatabase(t), wbModel(t)
	ctx := context.Background()
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accounts, err := identity.New(store.Pool())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.CreateUser(ctx, "", "admin", wbPassword, "ADMIN", true); err != nil {
		t.Fatal(err)
	}
	revision, err := knowledge.NewRevision(domain.RevisionInput{
		Title: "WB01 合成运维手册", Source: "fixture://wb01-browser",
		Content:       "# 指示灯\n合成测试设备的蓝灯表示维护模式。本资料为浏览器验收专用。",
		Applicability: domain.Applicability{Scope: "GENERAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CreateRevision(ctx, revision); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetPublication(ctx, revision.ID, "PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	files, err := blobstore.Open(filepath.Join(t.TempDir(), "private"), 500_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	app, err := application.New(store, cm, "REPLAY", application.Options{
		UsersMode: true, Files: files, Runner: wbRunner(t), AttachmentRunner: newAttachmentReplayRunner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewUnstartedServer(nil)
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	origin := "http://localhost:" + port
	static, _ := filepath.Abs("../../web/dist")
	handler, err := httpapi.NewUsers(app, accounts, httpapi.UsersConfig{Origin: origin, StaticDir: static})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	t.Logf("REPLAY browser fixture ready: %s pid=%d", origin, os.Getpid())
	stopped, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	select {
	case <-stopped.Done():
	case <-time.After(12 * time.Minute):
	}
}
