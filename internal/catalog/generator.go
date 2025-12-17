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
	force                   bool
}

func NewGenerator(migrationThreshold time.Duration, rootDir string, force bool) *Generator {
	registryCacheDir := filepath.Join(rootDir, ".cache", "registry-api")
	return &Generator{
		rootDir:                 rootDir,
		force:                   force,
		packageFactory:          NewPackageFactory(http.DefaultClient, migrationThreshold),
		buildpackRegistryClient: NewBuildpackRegistryClient(http.DefaultClient, registryCacheDir, force),
		indexer:                 NewIndexer(rootDir, force),
	}
}

func (g *Generator) GenerateCatalogFiles() error {
	err := g.indexer.Clone()
	if err != nil {
		return err
	}

	fmt.Printf("Scanning packages in: %s\n", g.rootDir)
	localPkgs, err := g.scanPackages(g.rootDir)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d packages\n", len(localPkgs))

	pkgs := g.filterPackages(localPkgs)

	registryCount := make(map[string]int)

	for _, pkg := range pkgs {
		registryCount[pkg.ImageRegistry()]++
		if _, ok := pkg.(*ECRPackage); ok {
			return fmt.Errorf("no ECR packages are candidates for migration")
		}
		if _, ok := pkg.(*GHCRPackage); ok {
			fmt.Printf("Artifact: %s/%s:%s GHCRPackage: %s/%s:%s\n", pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version(), pkg.ImageRepository(), pkg.ImageName(), pkg.Version())
		}
	}

	if len(registryCount) == 0 {
		fmt.Println("No packages will migrate (registries map is empty)")
		return nil
	}

	for registry, count := range registryCount {
		fmt.Printf("%s: %d\n", registry, count)
	}

	return nil
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

	fmt.Printf("Scanned %d files\n", fileCount)
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
	Namespace string `json:"ns"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Yanked    bool   `json:"yanked"`
	Addr      string `json:"addr"`
}

func (g *Generator) filterPackages(pkgs []Package) []Package {
	fmt.Printf("Starting to filter %d packages\n", len(pkgs))
	filteredPkgs := make([]Package, 0, len(pkgs))
	for i, pkg := range pkgs {
		if (i+1)%20 == 0 {
			fmt.Printf("Checking %d of %d packages. Remaining: %d\n", i+1, len(pkgs), len(pkgs)-i-1)
		}
		if pkg.WillMigrate(g.buildpackRegistryClient) {
			filteredPkgs = append(filteredPkgs, pkg)
		}
	}
	fmt.Printf("Finished filtering: %d packages will migrate\n", len(filteredPkgs))
	return filteredPkgs
}
