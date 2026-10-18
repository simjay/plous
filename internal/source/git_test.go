package source

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

func TestGitSnapshotWithoutExecutablesPreservesWorktree(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "app"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "app", "config.yaml")
	if err := os.WriteFile(file, []byte("kind: ConfigMap\n"), 0600); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("app/config.yaml"); err != nil {
		t.Fatal(err)
	}
	signature := &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}
	commit, err := worktree.Commit("fixture", &git.CommitOptions{Author: signature})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Storer.SetReference(plumbing.NewHashReference("refs/heads/staging", commit)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateTag("v1", commit, &git.CreateTagOptions{Tagger: signature, Message: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("uncommitted draft"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := worktree.Status()
	for _, revision := range []string{"", "staging", "refs/heads/staging", "v1", commit.String()} {
		snapshot, err := Load(context.Background(), Config{Repository: root, Revision: revision, Subdirectory: "app"})
		if err != nil {
			t.Fatalf("%q: %v", revision, err)
		}
		if snapshot.Revision != commit.String() || sourceTextFixture(t, snapshot, "config.yaml") != "kind: ConfigMap\n" {
			t.Fatalf("incorrect snapshot for %s", revision)
		}
	}
	rootSnapshot, err := Load(context.Background(), Config{Repository: root, Revision: commit.String(), Subdirectory: "app", RepositoryRoot: true, BrowsePath: "app"})
	if err != nil || sourceTextFixture(t, rootSnapshot, "app/config.yaml") != "kind: ConfigMap\n" || rootSnapshot.Has("config.yaml") {
		t.Fatal("root browsing retained cropped paths", err)
	}
	after, _ := worktree.Status()
	data, _ := os.ReadFile(file)
	if before.String() != after.String() || string(data) != "uncommitted draft" {
		t.Fatal("source checkout changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, Config{Repository: root}); err == nil {
		t.Fatal("cancelled source load succeeded")
	}
}

func TestHTTPSCredentialsAreHostScopedAndNative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	name := filepath.Join(home, ".netrc")
	text := "machine git.example.invalid login user password test-token\ndefault login shared password must-not-send\n"
	if err := os.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	method, err := httpsAuth("git.example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	auth, ok := method.(*githttp.BasicAuth)
	if !ok || auth.Username != "user" || auth.Password != "test-token" {
		t.Fatal("lost host-specific authentication")
	}
	if method, err := httpsAuth("another.invalid"); err != nil || method != nil {
		t.Fatal("credentials escaped their host")
	}
	after, _ := os.ReadFile(name)
	if string(after) != text {
		t.Fatal("credentials file changed")
	}
}

func TestGitHTTPSFetchWithoutExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "readme.txt"), []byte("captured commit"), 0600)
	worktree, _ := repository.Worktree()
	worktree.Add("readme.txt")
	commit, err := worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	repository.Storer.SetReference(plumbing.NewHashReference("refs/heads/staging", commit))
	repository.CreateTag("v1", commit, nil)
	endpoint, _ := transport.NewEndpoint("https://fixture.invalid/repo")
	backend := gitserver.NewServer(gitserver.MapLoader{endpoint.String(): repository.Storer})
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		user, password, ok := r.BasicAuth()
		if !ok || user != "fixture" || password != "private-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		session, err := backend.NewUploadPackSession(endpoint, nil)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer session.Close()
		refs, err := session.AdvertisedReferencesContext(r.Context())
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		// The fixture has one commit, so its complete pack is also a depth-one pack.
		refs.Capabilities.Set(capability.Shallow)
		refs.Capabilities.Set(capability.AllowReachableSHA1InWant)
		switch r.URL.Path {
		case "/repo/info/refs":
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			io.WriteString(w, "001e# service=git-upload-pack\n0000")
			if err := refs.Encode(w); err != nil {
				t.Error(err)
			}
		case "/repo/git-upload-pack":
			request := packp.NewUploadPackRequest()
			if err := request.Decode(r.Body); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			response, err := session.UploadPack(r.Context(), request)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			defer response.Close()
			w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
			if err := response.Encode(w); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previous := gitclient.Protocols["https"]
	gitclient.InstallProtocol("https", githttp.NewClient(server.Client()))
	t.Cleanup(func() { gitclient.InstallProtocol("https", previous) })
	os.WriteFile(filepath.Join(home, ".netrc"), []byte("machine 127.0.0.1 login fixture password private-token\n"), 0600)
	for _, revision := range []string{"", "staging", "v1", commit.String()} {
		snapshot, err := Load(context.Background(), Config{Repository: server.URL + "/repo", Revision: revision})
		if err != nil {
			t.Fatalf("%q: %v", revision, err)
		}
		if snapshot.Revision != commit.String() || sourceTextFixture(t, snapshot, "readme.txt") != "captured commit" {
			t.Fatal("remote snapshot mismatch")
		}
	}
	if requests.Load() < 8 {
		t.Fatal("fetch did not read the fixture")
	}
}

func TestGitFileToolsKeepAcquiredCommitAndSharedIgnoreRules(t *testing.T) {
	root := sourceFiles(t, map[string]string{"app.txt": "old revision\n", "ignored.txt": "hidden by ignore\n", "nested/keep.txt": "visible\n"})
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	signature := &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}
	if _, err := worktree.Commit("initial", &git.CommitOptions{Author: signature}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add(".gitignore"); err != nil {
		t.Fatal(err)
	}
	old, err := worktree.Commit("ignore rules", &git.CommitOptions{Author: signature})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := Load(t.Context(), Config{Repository: root})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.Files != nil || acquired.Has("ignored.txt") {
		t.Fatal("Git acquisition loaded bodies or ignored different paths")
	}
	scope := ReadScope{ProfileRoot: t.TempDir(), ConnectionID: "git", View: CaptureReadView(Discover(acquired), nil, nil, nil), Session: NewReadSession()}
	defer scope.Session.Close()
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("new revision\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("app.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("new", &git.CommitOptions{Author: signature}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := Load(t.Context(), Config{Repository: root})
	if err != nil {
		t.Fatal(err)
	}
	oldRead := readObservation(t, scope, ReadRequest{Operation: "read", Path: "app.txt"})
	if oldRead.Facts["excerpt"].String() != "old revision\n" || oldRead.Facts["revision"].String() != old.String() {
		t.Fatal("running read retargeted after refresh", oldRead)
	}
	scope.View = CaptureReadView(Discover(refreshed), nil, nil, nil)
	if result := readObservation(t, scope, ReadRequest{Operation: "read", Path: "app.txt"}); result.Facts["excerpt"].String() != "new revision\n" {
		t.Fatal(result)
	}
	if _, err := GuardedRead(t.Context(), scope, ReadRequest{Operation: "read", Path: "ignored.txt"}); err != nil {
		t.Fatal("exact Git read treated scanning exclusion as a permission", err)
	}
	reader, err := acquired.git.open("app.txt")
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- acquired.Cleanup()() }()
	select {
	case <-closed:
		t.Fatal("cleanup closed an active blob reader")
	case <-time.After(10 * time.Millisecond):
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "old revision\n" {
		t.Fatal("active Git reader lost its objects", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Git cleanup did not finish")
	}
	if _, err := acquired.git.open("app.txt"); err == nil {
		t.Fatal("closed Git handle reopened")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("cleanup removed the original repository", err)
	}

	local, err := Load(t.Context(), Config{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if local.Has("ignored.txt") || !local.Has("nested/keep.txt") {
		t.Fatal("local and Git acquisition disagree")
	}
}
