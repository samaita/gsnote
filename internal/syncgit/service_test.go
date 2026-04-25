package syncgit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestSyncRemoteAheadReturnsNothingToCommit(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")
	otherPath := cloneMain(t, remotePath, "other")

	writeFile(t, otherPath, "note.md", "remote update\n")
	commitAll(t, otherPath, "remote update")
	pushMain(t, otherPath)

	svc := New(localPath, "", "Test User", "test@example.com")
	outcome, err := svc.Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if outcome != OutcomeNothingToCommit {
		t.Fatalf("unexpected outcome: %v", outcome)
	}

	if got := readFile(t, localPath, "note.md"); got != "remote update\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestSyncDirtyWorktreeCreatesCommitAndPushes(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")

	writeFile(t, localPath, "daily.md", "fresh note\n")

	svc := New(localPath, "", "Test User", "test@example.com")
	outcome, err := svc.Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if outcome != OutcomeSynced {
		t.Fatalf("unexpected outcome: %v", outcome)
	}

	verifyPath := cloneMain(t, remotePath, "verify")
	if got := readFile(t, verifyPath, "daily.md"); got != "fresh note\n" {
		t.Fatalf("unexpected remote file content: %q", got)
	}
}

func TestSyncLocalAheadPushesExistingCommit(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")

	writeFile(t, localPath, "note.md", "local only\n")
	commitAll(t, localPath, "local only")

	svc := New(localPath, "", "Test User", "test@example.com")
	outcome, err := svc.Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if outcome != OutcomeSynced {
		t.Fatalf("unexpected outcome: %v", outcome)
	}

	verifyPath := cloneMain(t, remotePath, "verify")
	if got := readFile(t, verifyPath, "note.md"); got != "local only\n" {
		t.Fatalf("unexpected remote file content: %q", got)
	}
}

func TestSyncDivergedConflictRollsBack(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")
	otherPath := cloneMain(t, remotePath, "other")

	writeFile(t, localPath, "note.md", "local change\n")
	localCommit := commitAll(t, localPath, "local change")

	writeFile(t, otherPath, "note.md", "remote change\n")
	commitAll(t, otherPath, "remote change")
	pushMain(t, otherPath)

	svc := New(localPath, "", "Test User", "test@example.com")
	_, err := svc.Sync()
	if !errors.Is(err, ErrRebaseConflict) {
		t.Fatalf("expected rebase conflict, got %v", err)
	}

	repo := openRepo(t, localPath)
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.Hash() != localCommit {
		t.Fatalf("expected rollback to %s, got %s", localCommit, head.Hash())
	}
	if got := readFile(t, localPath, "note.md"); got != "local change\n" {
		t.Fatalf("unexpected restored content: %q", got)
	}
}

func TestSyncWIPConflictLeavesMarkers(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")
	otherPath := cloneMain(t, remotePath, "other")

	writeFile(t, localPath, "note.md", "wip change\n")

	writeFile(t, otherPath, "note.md", "remote change\n")
	commitAll(t, otherPath, "remote change")
	pushMain(t, otherPath)

	svc := New(localPath, "", "Test User", "test@example.com")
	_, err := svc.Sync()
	if !errors.Is(err, ErrStashConflict) {
		t.Fatalf("expected stash conflict, got %v", err)
	}

	content := readFile(t, localPath, "note.md")
	if !strings.Contains(content, "<<<<<<< WIP") {
		t.Fatalf("expected conflict markers, got %q", content)
	}
}

func TestSyncHTTPSRemoteRequiresToken(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	writeFile(t, repoPath, "note.md", "base\n")
	addAll(t, wt)
	hash, err := wt.Commit("init", &git.CommitOptions{Author: testAuthor()})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	setBranchHead(t, repo, hash)

	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{"https://github.com/example/private.git"},
	}); err != nil {
		t.Fatalf("create remote: %v", err)
	}

	svc := New(repoPath, "", "Test User", "test@example.com")
	_, err = svc.Sync()
	if err == nil || !strings.Contains(err.Error(), DefaultTokenEnvVar) {
		t.Fatalf("expected token validation error, got %v", err)
	}
}

func TestSyncRequiresAuthorEnvValues(t *testing.T) {
	remotePath := setupRemote(t, map[string]string{"note.md": "base\n"})
	localPath := cloneMain(t, remotePath, "local")

	svc := New(localPath, "", "", "")
	_, err := svc.Sync()
	if err == nil {
		t.Fatal("expected missing author error")
	}
	if !strings.Contains(err.Error(), DefaultAuthorNameEnv) || !strings.Contains(err.Error(), DefaultAuthorEmailEnv) {
		t.Fatalf("expected author env names in error, got %v", err)
	}
}

func setupRemote(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	remotePath := filepath.Join(root, "remote.git")
	if _, err := git.PlainInit(remotePath, true); err != nil {
		t.Fatalf("init bare remote: %v", err)
	}

	seedPath := filepath.Join(root, "seed")
	repo, err := git.PlainInit(seedPath, false)
	if err != nil {
		t.Fatalf("init seed repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("seed worktree: %v", err)
	}

	for name, content := range files {
		writeFile(t, seedPath, name, content)
	}
	addAll(t, wt)
	hash, err := wt.Commit("init", &git.CommitOptions{Author: testAuthor()})
	if err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	setBranchHead(t, repo, hash)

	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{remotePath},
	}); err != nil {
		t.Fatalf("seed create remote: %v", err)
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: DefaultRemoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/main:refs/heads/main"),
		},
	}); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	remoteRepo := openRepo(t, remotePath)
	if err := remoteRepo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(DefaultBranchName))); err != nil {
		t.Fatalf("set remote HEAD: %v", err)
	}

	return remotePath
}

func cloneMain(t *testing.T, remotePath, name string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	_, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           remotePath,
		ReferenceName: plumbing.NewBranchReferenceName(DefaultBranchName),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("clone %s: %v", name, err)
	}
	return dir
}

func openRepo(t *testing.T, path string) *git.Repository {
	t.Helper()

	repo, err := git.PlainOpen(path)
	if err != nil {
		t.Fatalf("open repo %s: %v", path, err)
	}
	return repo
}

func setBranchHead(t *testing.T, repo *git.Repository, hash plumbing.Hash) {
	t.Helper()

	branchRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(DefaultBranchName), hash)
	if err := repo.Storer.SetReference(branchRef); err != nil {
		t.Fatalf("set branch ref: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(DefaultBranchName))); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
}

func commitAll(t *testing.T, repoPath, message string) plumbing.Hash {
	t.Helper()

	repo := openRepo(t, repoPath)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	addAll(t, wt)
	hash, err := wt.Commit(message, &git.CommitOptions{Author: testAuthor()})
	if err != nil {
		t.Fatalf("commit %q: %v", message, err)
	}
	return hash
}

func testAuthor() *object.Signature {
	return &object.Signature{
		Name:  "Test User",
		Email: "test@example.com",
		When:  time.Now(),
	}
}

func pushMain(t *testing.T, repoPath string) {
	t.Helper()

	repo := openRepo(t, repoPath)
	if err := repo.Push(&git.PushOptions{
		RemoteName: DefaultRemoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/main:refs/heads/main"),
		},
	}); err != nil {
		t.Fatalf("push main: %v", err)
	}
}

func addAll(t *testing.T, wt *git.Worktree) {
	t.Helper()

	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatalf("add all tracked: %v", err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatalf("add glob: %v", err)
	}
}

func writeFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFile(t *testing.T, root, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(data)
}
