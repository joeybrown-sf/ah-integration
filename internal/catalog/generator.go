package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Generator struct {
	packageFactory          *PackageFactory
	buildpackRegistryClient *BuildpackRegistryClient
	indexer                 *Indexer
	rootDir                 string
	outputWriter            OutputWriter
}

func NewGenerator(migrationThreshold time.Duration, rootDir string, outputWriter OutputWriter) *Generator {
	registryCacheDir := filepath.Join(rootDir, "registry-api-cache")
	return &Generator{
		packageFactory:          NewPackageFactory(http.DefaultClient, migrationThreshold),
		buildpackRegistryClient: NewBuildpackRegistryClient(http.DefaultClient, registryCacheDir),
		indexer:                 NewIndexer(),
		rootDir:                 rootDir,
		outputWriter:            outputWriter,
	}
}

func (g *Generator) GenerateCatalogFiles(force bool, namespaces []string, ignoredRegistries []string) error {
	// Clone https://github.com/buildpacks/registry-index.git
	repoDir, err := g.indexer.Clone(g.rootDir, force)
	if err != nil {
		return err
	}

	// Scan the packages in the index
	localPkgs, err := g.scanPackages(repoDir)
	if err != nil {
		return err
	}

	// Filter down to only packages that are candidates for migration
	pkgs := g.filterPackages(localPkgs, namespaces, ignoredRegistries)

	// // Count registries
	// registryCount := make(map[string]int)
	// for _, pkg := range pkgs {
	// 	registryCount[pkg.ImageRegistry()]++
	// }
	// if len(registryCount) == 0 {
	// 	fmt.Println("No packages will migrate (registries map is empty)")
	// 	return nil
	// }
	// for registry, count := range registryCount {
	// 	fmt.Printf("%s: %d\n", registry, count)
	// }

	return g.outputWriter.Write(pkgs, force)
}

func (g *Generator) scanPackages(path string) ([]Package, error) {
	var allPkgs []Package
	fileCount := 0

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			err := filepath.Walk(filepath.Join(path, entry.Name()), func(filePath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}

				if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
					return nil
				}

				fileCount++
				pkgs, err := g.parsePackageJsonL(filePath)
				if err != nil {
					fmt.Printf("failed to parse %s: %v\n", filePath, err)
					return nil
				}
				allPkgs = append(allPkgs, pkgs...)

				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}

	fmt.Printf("Found %d index files in %s\n", fileCount, path)
	return allPkgs, nil
}

func (g *Generator) parsePackageJsonL(path string) ([]Package, error) {
	jsonl, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(jsonl), "\n")

	var pkgs []Package

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var rawPkg rawPackage
		err = json.Unmarshal([]byte(line), &rawPkg)
		if err != nil {
			fmt.Printf("failed to parse JSON line in %s: %v\n", path, err)
			continue
		}

		pkg, err := g.packageFactory.CreatePackage(rawPkg.Namespace, rawPkg.Name, rawPkg)
		if err != nil {
			fmt.Printf("failed to create package from %s: %v\n", path, err)
			continue
		}

		pkgs = append(pkgs, pkg)
	}

	return pkgs, nil
}

type rawPackage struct {
	Namespace   string    `json:"ns"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description"`
	Yanked      bool      `json:"yanked"`
	Addr        string    `json:"addr"`
	Licenses    []string  `json:"licenses"`
	Homepage    string    `json:"homepage"`
}

func (g *Generator) filterByNamespaces(pkgs []Package, namespaces []string) []Package {
	if len(namespaces) == 0 {
		return pkgs
	}

	namespaceMap := make(map[string]bool)
	for _, ns := range namespaces {
		namespaceMap[ns] = true
	}

	filteredPkgs := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if namespaceMap[pkg.ArtifactRepository()] {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	return filteredPkgs
}

func (g *Generator) filterByIgnoredRegistries(pkgs []Package, ignoredRegistries []string) []Package {
	if len(ignoredRegistries) == 0 {
		return pkgs
	}

	registryMap := make(map[string]bool)
	for _, reg := range ignoredRegistries {
		registryMap[reg] = true
	}

	filteredPkgs := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if !registryMap[pkg.ImageRegistry()] {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	return filteredPkgs
}

func (g *Generator) filterPackages(pkgs []Package, namespaces []string, ignoredRegistries []string) []Package {
	if len(namespaces) > 0 {
		pkgs = g.filterByNamespaces(pkgs, namespaces)
	}

	if len(ignoredRegistries) > 0 {
		pkgs = g.filterByIgnoredRegistries(pkgs, ignoredRegistries)
	}

	filteredPkgs := make([]Package, 0, len(pkgs))
	for i, pkg := range pkgs {
		if (i+1)%20 == 0 {
			fmt.Printf("Pulling package metadata from registry-api: %d of %d packages. Remaining: %d\n", i+1, len(pkgs), len(pkgs)-i-1)
		}
		
		// If CreatedAt is zero, try to get it from registry metadata
		if pkg.CreatedAt().IsZero() {
			// Try to get metadata to populate CreatedAt
			// We need to access the artifact's GetBuildpackRegistryMetadata method
			// Use type assertion to check if it's one of our package types
			switch p := pkg.(type) {
			case *DockerhubPackage:
				if metadata, exists := p.GetBuildpackRegistryMetadata(g.buildpackRegistryClient, false); exists && !metadata.CreatedAt.IsZero() {
					p.SetCreatedAt(metadata.CreatedAt)
				}
			case *GHCRPackage:
				if metadata, exists := p.GetBuildpackRegistryMetadata(g.buildpackRegistryClient, false); exists && !metadata.CreatedAt.IsZero() {
					p.SetCreatedAt(metadata.CreatedAt)
				}
			case *ECRPackage:
				if metadata, exists := p.GetBuildpackRegistryMetadata(g.buildpackRegistryClient, false); exists && !metadata.CreatedAt.IsZero() {
					p.SetCreatedAt(metadata.CreatedAt)
				}
			case *GCRPackage:
				if metadata, exists := p.GetBuildpackRegistryMetadata(g.buildpackRegistryClient, false); exists && !metadata.CreatedAt.IsZero() {
					p.SetCreatedAt(metadata.CreatedAt)
				}
			case *FuturehaxPackage:
				if metadata, exists := p.GetBuildpackRegistryMetadata(g.buildpackRegistryClient, false); exists && !metadata.CreatedAt.IsZero() {
					p.SetCreatedAt(metadata.CreatedAt)
				}
			}
		}
		
		if pkg.WillMigrate(g.buildpackRegistryClient) {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	return filteredPkgs
}
