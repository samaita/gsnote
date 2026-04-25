package syncgit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	transport "github.com/go-git/go-git/v5/plumbing/transport"
	httptransport "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const (
	DefaultRemoteName     = "origin"
	DefaultBranchName     = "main"
	DefaultCommitMsg      = "Sync from telegram"
	DefaultTokenEnvVar    = "GSNOTE_GITHUB_TOKEN"
	DefaultAuthorNameEnv  = "GSNOTE_GIT_AUTHOR_NAME"
	DefaultAuthorEmailEnv = "GSNOTE_GIT_AUTHOR_EMAIL"
)

var (
	ErrFetch          = errors.New("sync fetch failed")
	ErrRebaseConflict = errors.New("sync rebase conflict")
	ErrStashConflict  = errors.New("sync stash restore conflict")
	ErrAdd            = errors.New("sync add failed")
	ErrCommit         = errors.New("sync commit failed")
	ErrPush           = errors.New("sync push failed")
)

type Outcome int

const (
	OutcomeSynced Outcome = iota
	OutcomeNothingToCommit
)

type Service struct {
	root         string
	remoteName   string
	branchName   string
	commitMsg    string
	token        string
	tokenEnvName string
	authorName   string
	authorEmail  string
}

func New(root, token, authorName, authorEmail string) *Service {
	return &Service{
		root:         root,
		remoteName:   DefaultRemoteName,
		branchName:   DefaultBranchName,
		commitMsg:    DefaultCommitMsg,
		token:        token,
		tokenEnvName: DefaultTokenEnvVar,
		authorName:   authorName,
		authorEmail:  authorEmail,
	}
}

type syncState struct {
	repo        *git.Repository
	worktree    *git.Worktree
	branchRef   plumbing.ReferenceName
	branchHash  plumbing.Hash
	remoteHash  plumbing.Hash
	origHash    plumbing.Hash
	remoteURL   string
	auth        transport.AuthMethod
	author      object.Signature
	wip         *wipState
	branchAhead bool
}

type wipState struct {
	refName    plumbing.ReferenceName
	commitHash plumbing.Hash
	baseHash   plumbing.Hash
}

type fileSnapshot struct {
	mode    filemode.FileMode
	content []byte
}

func (s *Service) Sync() (Outcome, error) {
	st, err := s.open()
	if err != nil {
		return OutcomeNothingToCommit, err
	}

	cleanupWIP := func() {
		if st.wip == nil {
			return
		}
		_ = st.repo.Storer.RemoveReference(st.wip.refName)
	}
	defer cleanupWIP()

	if st.wip, err = s.captureWIP(st); err != nil {
		return OutcomeNothingToCommit, err
	}

	if err := s.fetchRemote(st); err != nil {
		_ = s.restoreOriginalState(st)
		return OutcomeNothingToCommit, err
	}

	if err := s.alignWithRemote(st); err != nil {
		_ = s.restoreOriginalState(st)
		return OutcomeNothingToCommit, err
	}

	if err := s.restoreWIP(st); err != nil {
		return OutcomeNothingToCommit, err
	}

	if err := s.stageAll(st.worktree); err != nil {
		return OutcomeNothingToCommit, fmt.Errorf("%w: %v", ErrAdd, err)
	}

	createdCommit := false
	if _, err := st.worktree.Commit(s.commitMsg, &git.CommitOptions{Author: &st.author}); err != nil {
		if !errors.Is(err, git.ErrEmptyCommit) {
			return OutcomeNothingToCommit, fmt.Errorf("%w: %v", ErrCommit, err)
		}
	} else {
		createdCommit = true
		st.branchAhead = true
	}

	if !st.branchAhead {
		return OutcomeNothingToCommit, nil
	}

	if err := s.push(st); err != nil {
		return OutcomeNothingToCommit, err
	}

	if createdCommit || st.wip != nil || st.branchAhead {
		return OutcomeSynced, nil
	}

	return OutcomeNothingToCommit, nil
}

func (s *Service) open() (*syncState, error) {
	repo, err := git.PlainOpen(s.root)
	if err != nil {
		return nil, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	expectedBranch := plumbing.NewBranchReferenceName(s.branchName)
	if head.Name() != expectedBranch {
		return nil, fmt.Errorf("sync expects branch %s, found %s", expectedBranch.Short(), head.Name().Short())
	}

	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}

	remoteCfg, ok := cfg.Remotes[s.remoteName]
	if !ok || len(remoteCfg.URLs) == 0 {
		return nil, fmt.Errorf("missing remote %s", s.remoteName)
	}

	author, err := s.authorSignature()
	if err != nil {
		return nil, err
	}

	auth, err := s.authForRemote(remoteCfg.URLs[0])
	if err != nil {
		return nil, err
	}

	return &syncState{
		repo:       repo,
		worktree:   worktree,
		branchRef:  head.Name(),
		branchHash: head.Hash(),
		origHash:   head.Hash(),
		remoteURL:  remoteCfg.URLs[0],
		auth:       auth,
		author:     author,
	}, nil
}

func (s *Service) authorSignature() (object.Signature, error) {
	name := strings.TrimSpace(s.authorName)
	email := strings.TrimSpace(s.authorEmail)
	if name == "" || email == "" {
		return object.Signature{}, fmt.Errorf("%s and %s are required for sync commits", DefaultAuthorNameEnv, DefaultAuthorEmailEnv)
	}

	return object.Signature{
		Name:  name,
		Email: email,
		When:  time.Now(),
	}, nil
}

func (s *Service) authForRemote(remoteURL string) (transport.AuthMethod, error) {
	if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		if strings.TrimSpace(s.token) == "" {
			return nil, fmt.Errorf("%s is required for HTTPS sync remotes", s.tokenEnvName)
		}

		return &httptransport.BasicAuth{
			Username: "x-access-token",
			Password: s.token,
		}, nil
	}

	return nil, nil
}

func (s *Service) captureWIP(st *syncState) (*wipState, error) {
	status, err := st.worktree.Status()
	if err != nil {
		return nil, err
	}
	if status.IsClean() {
		return nil, nil
	}

	refName := plumbing.NewBranchReferenceName(fmt.Sprintf("gsnote-sync-wip-%d", time.Now().UnixNano()))
	if err := st.worktree.Checkout(&git.CheckoutOptions{
		Hash:   st.origHash,
		Branch: refName,
		Create: true,
		Keep:   true,
	}); err != nil {
		return nil, err
	}

	if err := s.stageAll(st.worktree); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdd, err)
	}

	commitHash, err := st.worktree.Commit("gsnote temporary sync wip", &git.CommitOptions{Author: &st.author})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCommit, err)
	}

	if err := st.worktree.Checkout(&git.CheckoutOptions{
		Branch: st.branchRef,
		Force:  true,
	}); err != nil {
		return nil, err
	}

	if err := st.worktree.Reset(&git.ResetOptions{
		Commit: st.origHash,
		Mode:   git.HardReset,
	}); err != nil {
		return nil, err
	}

	return &wipState{
		refName:    refName,
		commitHash: commitHash,
		baseHash:   st.origHash,
	}, nil
}

func (s *Service) fetchRemote(st *syncState) error {
	fetchOpts := &git.FetchOptions{
		RemoteName: s.remoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", s.branchName, s.remoteName, s.branchName)),
		},
	}
	if st.auth != nil {
		fetchOpts.Auth = st.auth
	}

	if err := st.repo.Fetch(fetchOpts); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("%w: %v", ErrFetch, err)
	}

	remoteRef, err := st.repo.Reference(plumbing.NewRemoteReferenceName(s.remoteName, s.branchName), true)
	if err != nil {
		return fmt.Errorf("%w: remote ref not found: %v", ErrFetch, err)
	}

	st.remoteHash = remoteRef.Hash()
	return nil
}

func (s *Service) alignWithRemote(st *syncState) error {
	switch {
	case st.branchHash == st.remoteHash:
		st.branchAhead = false
		return nil
	case isAncestor(st.repo, st.branchHash, st.remoteHash):
		if err := st.worktree.Reset(&git.ResetOptions{
			Commit: st.remoteHash,
			Mode:   git.HardReset,
		}); err != nil {
			return err
		}
		st.branchHash = st.remoteHash
		st.branchAhead = false
		return nil
	case isAncestor(st.repo, st.remoteHash, st.branchHash):
		st.branchAhead = true
		return nil
	default:
		return s.rebaseEquivalent(st)
	}
}

func (s *Service) rebaseEquivalent(st *syncState) error {
	baseHash, ok := findMergeBase(st.repo, st.branchHash, st.remoteHash)
	if !ok {
		return fmt.Errorf("%w: merge-base not found", ErrRebaseConflict)
	}

	baseTree, err := treeByCommit(st.repo, baseHash)
	if err != nil {
		return err
	}
	localTree, err := treeByCommit(st.repo, st.branchHash)
	if err != nil {
		return err
	}
	remoteTree, err := treeByCommit(st.repo, st.remoteHash)
	if err != nil {
		return err
	}

	merged, conflicts, err := mergeTrees(baseTree, localTree, remoteTree)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("%w: conflicting paths: %s", ErrRebaseConflict, strings.Join(conflicts, ", "))
	}

	if err := st.worktree.Reset(&git.ResetOptions{
		Commit: st.remoteHash,
		Mode:   git.HardReset,
	}); err != nil {
		return err
	}

	if err := applyTreeToWorktree(s.root, remoteTree, merged); err != nil {
		return err
	}
	if err := s.stageAll(st.worktree); err != nil {
		return fmt.Errorf("%w: %v", ErrAdd, err)
	}

	if _, err := st.worktree.Commit("Reapply local changes after syncing with origin/main", &git.CommitOptions{Author: &st.author}); err != nil {
		if errors.Is(err, git.ErrEmptyCommit) {
			st.branchHash = st.remoteHash
			st.branchAhead = false
			return nil
		}
		return fmt.Errorf("%w: %v", ErrCommit, err)
	}

	head, err := st.repo.Head()
	if err != nil {
		return err
	}
	st.branchHash = head.Hash()
	st.branchAhead = true
	return nil
}

func (s *Service) restoreWIP(st *syncState) error {
	if st.wip == nil {
		return nil
	}

	baseTree, err := treeByCommit(st.repo, st.wip.baseHash)
	if err != nil {
		return err
	}
	wipTree, err := treeByCommit(st.repo, st.wip.commitHash)
	if err != nil {
		return err
	}

	head, err := st.repo.Head()
	if err != nil {
		return err
	}
	currentTree, err := treeByCommit(st.repo, head.Hash())
	if err != nil {
		return err
	}

	merged, conflicts, err := mergeTrees(baseTree, wipTree, currentTree)
	if err != nil {
		return err
	}

	if len(conflicts) == 0 {
		return applyTreeToWorktree(s.root, currentTree, merged)
	}

	if err := applyTreeToWorktree(s.root, currentTree, merged); err != nil {
		return err
	}
	if err := writeConflictMarkers(s.root, conflicts, baseTree, wipTree, currentTree); err != nil {
		return err
	}

	return fmt.Errorf("%w: conflicting paths: %s", ErrStashConflict, strings.Join(conflicts, ", "))
}

func (s *Service) push(st *syncState) error {
	pushOpts := &git.PushOptions{
		RemoteName: s.remoteName,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", s.branchName, s.branchName)),
		},
	}
	if st.auth != nil {
		pushOpts.Auth = st.auth
	}

	if err := st.repo.Push(pushOpts); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("%w: %v", ErrPush, err)
	}

	return nil
}

func (s *Service) restoreOriginalState(st *syncState) error {
	if err := st.worktree.Reset(&git.ResetOptions{
		Commit: st.origHash,
		Mode:   git.HardReset,
	}); err != nil {
		return err
	}

	if st.wip == nil {
		return nil
	}

	baseTree, err := treeByCommit(st.repo, st.wip.baseHash)
	if err != nil {
		return err
	}
	wipTree, err := treeByCommit(st.repo, st.wip.commitHash)
	if err != nil {
		return err
	}
	currentTree, err := treeByCommit(st.repo, st.origHash)
	if err != nil {
		return err
	}

	merged, _, err := mergeTrees(baseTree, wipTree, currentTree)
	if err != nil {
		return err
	}

	return applyTreeToWorktree(s.root, currentTree, merged)
}

func (s *Service) stageAll(worktree *git.Worktree) error {
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	if err := worktree.AddGlob("."); err != nil {
		return err
	}
	return nil
}

func treeByCommit(repo *git.Repository, hash plumbing.Hash) (map[string]fileSnapshot, error) {
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	files := make(map[string]fileSnapshot)
	err = tree.Files().ForEach(func(f *object.File) error {
		reader, err := f.Reader()
		if err != nil {
			return err
		}
		defer reader.Close()

		content, err := readAll(reader)
		if err != nil {
			return err
		}

		files[f.Name] = fileSnapshot{
			mode:    f.Mode,
			content: content,
		}
		return nil
	})
	return files, err
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

func mergeTrees(base, local, remote map[string]fileSnapshot) (map[string]fileSnapshot, []string, error) {
	result := make(map[string]fileSnapshot)
	conflicts := make([]string, 0)

	paths := make(map[string]struct{})
	for path := range base {
		paths[path] = struct{}{}
	}
	for path := range local {
		paths[path] = struct{}{}
	}
	for path := range remote {
		paths[path] = struct{}{}
	}

	for path := range paths {
		baseFile, hasBase := base[path]
		localFile, hasLocal := local[path]
		remoteFile, hasRemote := remote[path]

		switch {
		case snapshotsEqual(hasLocal, localFile, hasRemote, remoteFile):
			if hasLocal {
				result[path] = localFile
			}
		case snapshotsEqual(hasBase, baseFile, hasLocal, localFile):
			if hasRemote {
				result[path] = remoteFile
			}
		case snapshotsEqual(hasBase, baseFile, hasRemote, remoteFile):
			if hasLocal {
				result[path] = localFile
			}
		default:
			conflicts = append(conflicts, path)
			if hasRemote {
				result[path] = remoteFile
			}
		}
	}

	slices.Sort(conflicts)
	return result, conflicts, nil
}

func snapshotsEqual(hasA bool, a fileSnapshot, hasB bool, b fileSnapshot) bool {
	if hasA != hasB {
		return false
	}
	if !hasA {
		return true
	}
	return a.mode == b.mode && bytes.Equal(a.content, b.content)
}

func applyTreeToWorktree(root string, current, desired map[string]fileSnapshot) error {
	for path := range current {
		if _, ok := desired[path]; ok {
			continue
		}
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	for path, file := range desired {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}

		switch file.mode {
		case filemode.Symlink:
			if err := os.RemoveAll(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Symlink(string(file.content), fullPath); err != nil {
				return err
			}
		default:
			mode, err := file.mode.ToOSFileMode()
			if err != nil {
				return err
			}
			if err := os.RemoveAll(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.WriteFile(fullPath, file.content, mode.Perm()); err != nil {
				return err
			}
			if err := os.Chmod(fullPath, mode.Perm()); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeConflictMarkers(root string, conflicts []string, base, local, remote map[string]fileSnapshot) error {
	for _, path := range conflicts {
		localFile, hasLocal := local[path]
		remoteFile, hasRemote := remote[path]
		baseFile, hasBase := base[path]

		if !hasLocal || !hasRemote || localFile.mode == filemode.Symlink || remoteFile.mode == filemode.Symlink {
			continue
		}

		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}

		baseContent := ""
		if hasBase {
			baseContent = string(baseFile.content)
		}

		content := fmt.Sprintf("<<<<<<< WIP\n%s\n||||||| BASE\n%s\n=======\n%s\n>>>>>>> REMOTE\n", string(localFile.content), baseContent, string(remoteFile.content))
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func isAncestor(repo *git.Repository, ancestor, descendant plumbing.Hash) bool {
	if ancestor == descendant {
		return true
	}

	seen := map[plumbing.Hash]bool{}
	queue := []plumbing.Hash{descendant}
	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]
		if seen[hash] {
			continue
		}
		seen[hash] = true
		if hash == ancestor {
			return true
		}

		commit, err := repo.CommitObject(hash)
		if err != nil {
			continue
		}
		_ = commit.Parents().ForEach(func(parent *object.Commit) error {
			queue = append(queue, parent.Hash)
			return nil
		})
	}

	return false
}

func findMergeBase(repo *git.Repository, a, b plumbing.Hash) (plumbing.Hash, bool) {
	aDepths := ancestorDepths(repo, a)
	bDepths := ancestorDepths(repo, b)

	best := plumbing.ZeroHash
	bestDistance := int(^uint(0) >> 1)
	for hash, aDepth := range aDepths {
		bDepth, ok := bDepths[hash]
		if !ok {
			continue
		}
		if d := aDepth + bDepth; d < bestDistance {
			best = hash
			bestDistance = d
		}
	}

	return best, !best.IsZero()
}

func ancestorDepths(repo *git.Repository, start plumbing.Hash) map[plumbing.Hash]int {
	depths := map[plumbing.Hash]int{start: 0}
	queue := []plumbing.Hash{start}

	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]

		commit, err := repo.CommitObject(hash)
		if err != nil {
			continue
		}
		currentDepth := depths[hash]
		_ = commit.Parents().ForEach(func(parent *object.Commit) error {
			if prev, ok := depths[parent.Hash]; ok && prev <= currentDepth+1 {
				return nil
			}
			depths[parent.Hash] = currentDepth + 1
			queue = append(queue, parent.Hash)
			return nil
		})
	}

	return depths
}
