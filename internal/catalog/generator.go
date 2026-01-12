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

func (g *Generator) GenerateCatalogFiles(force bool, namespaces []string, registries []string, names []string) error {
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
	pkgs := g.filterAndEnrichPackages(localPkgs, namespaces, registries, names)

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

func (g *Generator) filterByRegistries(pkgs []Package, registries []string) []Package {
	if len(registries) == 0 {
		return pkgs
	}

	registryMap := make(map[string]bool)
	for _, reg := range registries {
		registryMap[reg] = true
	}

	filteredPkgs := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if registryMap[pkg.ImageRegistry()] {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	return filteredPkgs
}

func (g *Generator) filterByNames(pkgs []Package, names []string) []Package {
	if len(names) == 0 {
		return pkgs
	}

	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	filteredPkgs := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if nameMap[pkg.ArtifactName()] {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	return filteredPkgs
}

func (g *Generator) filterAndEnrichPackages(pkgs []Package, namespaces []string, registries []string, names []string) []Package {
	if len(namespaces) > 0 {
		pkgs = g.filterByNamespaces(pkgs, namespaces)
	}
	if len(registries) > 0 {
		pkgs = g.filterByRegistries(pkgs, registries)
	}
	if len(names) > 0 {
		pkgs = g.filterByNames(pkgs, names)
	}

	enrichedPkgs := []Package{}
	for i, pkg := range pkgs {
		if pkg.Yanked() {
			continue
		}

		if (i+1)%20 == 0 {
			fmt.Printf("Pulling package metadata from registry-api: %d of %d packages. Remaining: %d\n", i+1, len(pkgs), len(pkgs)-i-1)
		}
		bprMetadata, err := g.buildpackRegistryClient.GetBuildpackRegistryMetadata(pkg, false)
		if err != nil {
			fmt.Printf("failed to get buildpack registry for %s/%s: %v\n", pkg.ImageRepository(), pkg.ImageName(), err)
			continue
		}

		pkg.SetDescription(bprMetadata.Description)

		if bprMetadata.Description != "" {
			pkg.SetDescription(bprMetadata.Description)
		}

		pkg.SetLicenses(bprMetadata.Licenses)
		pkg.SetHomepage(bprMetadata.Homepage)
		pkg.SetCreatedAt(bprMetadata.CreatedAt)

		if !pkg.WillMigrate() {
			continue
		}

		enrichedPkgs = append(enrichedPkgs, pkg)
	}
	return enrichedPkgs
}
