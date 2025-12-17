package catalog

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type Indexer struct {
	rootDir    string
	githubRepo string
	force      bool
}

func NewIndexer(rootDir string, force bool) *Indexer {
	return &Indexer{
		rootDir:    rootDir,
		githubRepo: "https://github.com/buildpacks/registry-index.git",
		force:      force,
	}
}

func (i *Indexer) Clone() error {
	anyContentExists, err := ensureDirExists(i.rootDir)
	if err != nil {
		return err
	}

	if !i.force && anyContentExists {
		return nil
	}

	// clone the github repo into the root dir
	repo, err := git.PlainClone(i.rootDir, false, &git.CloneOptions{
		URL:          i.githubRepo,
		SingleBranch: true,
		Depth:        1,
		NoCheckout:   true,
	})
	if err != nil {
		return err
	}

	// Get the remote to find the default branch
	remotes, err := repo.Remotes()
	if err != nil {
		return err
	}
	if len(remotes) == 0 {
		return fmt.Errorf("no remotes found")
	}

	// Get the HEAD reference from the remote
	remoteRefs, err := remotes[0].List(&git.ListOptions{})
	if err != nil {
		return err
	}

	var headRef *plumbing.Reference
	for _, ref := range remoteRefs {
		if ref.Name() == plumbing.HEAD {
			headRef = ref
			break
		}
	}

	if headRef == nil {
		return fmt.Errorf("could not find HEAD reference")
	}

	// Resolve the branch reference (HEAD points to refs/heads/main or similar)
	branchRefName := headRef.Target()
	if branchRefName == "" {
		// Fallback to main if HEAD doesn't have a target
		branchRefName = plumbing.ReferenceName("refs/heads/main")
	}

	// Get the actual branch reference
	var branchRef *plumbing.Reference
	for _, ref := range remoteRefs {
		if ref.Name() == branchRefName {
			branchRef = ref
			break
		}
	}

	if branchRef == nil {
		return fmt.Errorf("could not find branch reference: %s", branchRefName)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Checkout using the hash from the branch reference
	err = worktree.Checkout(&git.CheckoutOptions{
		Hash:  branchRef.Hash(),
		Force: true,
	})
	if err != nil {
		return err
	}

	return nil
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
