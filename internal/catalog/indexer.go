package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type Indexer struct {
	githubRepo    string
	branchRefName plumbing.ReferenceName
}

func NewIndexer() *Indexer {
	return &Indexer{
		githubRepo:    "https://github.com/buildpacks/registry-index.git",
		branchRefName: plumbing.ReferenceName("refs/heads/main"),
	}
}

// Clone the github repo into the root dir.
// Force will overwrite the existing content if it exists.
func (i *Indexer) Clone(rootDir string, force bool) (string, error) {
	repoDir := filepath.Join(rootDir, "registry-index")

	anyContentExists, err := ensureDirExists(repoDir)
	if err != nil {
		return "", err
	}

	if !force && anyContentExists {
		return repoDir, nil
	}

	repo, err := git.PlainClone(repoDir, false, &git.CloneOptions{
		URL:          i.githubRepo,
		SingleBranch: true,
		Depth:        1,
		NoCheckout:   true,
	})
	if err != nil {
		return "", err
	}

	remotes, err := repo.Remotes()
	if err != nil {
		return "", err
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("no remotes found")
	}

	remoteRefs, err := remotes[0].List(&git.ListOptions{})
	if err != nil {
		return "", err
	}

	var headRef *plumbing.Reference
	for _, ref := range remoteRefs {
		if ref.Name() == plumbing.HEAD {
			headRef = ref
			break
		}
	}

	if headRef == nil {
		return "", fmt.Errorf("could not find HEAD reference")
	}

	branchRefName := headRef.Target()
	if branchRefName == "" {
		branchRefName = i.branchRefName
	}

	var branchRef *plumbing.Reference
	for _, ref := range remoteRefs {
		if ref.Name() == branchRefName {
			branchRef = ref
			break
		}
	}

	if branchRef == nil {
		return "", fmt.Errorf("could not find branch reference: %s", branchRefName)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash:  branchRef.Hash(),
		Force: true,
	})
	if err != nil {
		return "", err
	}

	return repoDir, nil
}

func ensureDirExists(dir string) (bool, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return false, err
		}
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}
