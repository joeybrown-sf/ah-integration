package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

type OutputWriter interface {
	Write(pkgs []Package, force bool) error
}

type FilesystemOutputWriter struct {
	outputDir      string
	imageExtractor *ImageExtractor
}

// Change represents a change introduced in a package version.
type Change struct {
	Kind        string  `json:"kind,omitempty"`
	Description string  `json:"description"`
	Links       []*Link `json:"links,omitempty"`
}

type Link struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url" yaml:"url"`
}

type ContainerImage struct {
	Name        string   `json:"name" yaml:"name"`
	Image       string   `json:"image" yaml:"image"`
	Whitelisted bool     `json:"whitelisted" yaml:"whitelisted"`
	Platforms   []string `json:"platforms" yaml:"platforms"`
}

// Maintainer represents a package's maintainer.
type Maintainer struct {
	MaintainerID string `json:"maintainer_id"`
	Name         string `json:"name" yaml:"name"`
	Email        string `json:"email" yaml:"email"`
}

// Provider represents a package's provider.
type Provider struct {
	Name string `yaml:"name"`
}

// Recommendation represents some information about a recommended package.
type Recommendation struct {
	URL string `json:"url" yaml:"url"`
}

// Screenshot represents a screenshot associated with a package.
type Screenshot struct {
	Title string `json:"title" yaml:"title"`
	URL   string `json:"url" yaml:"url"`
}

type PackageMetadata struct {
	Version                 string            `yaml:"version"`
	Name                    string            `yaml:"name"`
	AlternativeName         string            `yaml:"alternativeName"`
	Category                string            `yaml:"category"`
	DisplayName             string            `yaml:"displayName"`
	CreatedAt               string            `yaml:"createdAt,omitempty"`
	Description             string            `yaml:"description"`
	LogoPath                string            `yaml:"logoPath"`
	LogoURL                 string            `yaml:"logoURL"`
	Digest                  string            `yaml:"digest,omitempty"`
	License                 string            `yaml:"license,omitempty"`
	HomeURL                 string            `yaml:"homeURL"`
	AppVersion              string            `yaml:"appVersion"`
	PublisherID             string            `yaml:"publisherID"`
	ContainersImages        []*ContainerImage `yaml:"containersImages"`
	Operator                bool              `yaml:"operator"`
	Deprecated              bool              `yaml:"deprecated"`
	Keywords                []string          `yaml:"keywords"`
	Links                   []*Link           `yaml:"links"`
	Readme                  string            `yaml:"readme"`
	Install                 string            `yaml:"install"`
	Changes                 []*Change         `yaml:"changes"`
	ContainsSecurityUpdates bool              `yaml:"containsSecurityUpdates"`
	Prerelease              bool              `yaml:"prerelease"`
	Maintainers             []*Maintainer     `yaml:"maintainers"`
	Provider                *Provider         `yaml:"provider"`
	Ignore                  []string          `yaml:"ignore"`
	Recommendations         []*Recommendation `yaml:"recommendations"`
	Screenshots             []*Screenshot     `yaml:"screenshots"`
	Annotations             map[string]string `yaml:"annotations"`
}

func (w *FilesystemOutputWriter) Write(pkgs []Package, force bool) error {
	for i, pkg := range pkgs {
		versionDir := filepath.Join(w.outputDir, pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())
		artifacthubPkgYml := filepath.Join(versionDir, "artifacthub-pkg.yml")
		buildpackToml := filepath.Join(versionDir, "buildpack.toml")
		packageToml := filepath.Join(versionDir, "package.toml")

		// Check if we should skip this package
		stat, err := os.Stat(artifacthubPkgYml)
		if err == nil {
			// File exists, check if we should skip it
			if !force && stat.Size() > 0 {
				// Also check if both toml files exist
				buildpackStat, _ := os.Stat(buildpackToml)
				packageStat, _ := os.Stat(packageToml)
				if buildpackStat != nil && packageStat != nil {
					// All files exist, skip
					continue
				}
			}
		} else if !os.IsNotExist(err) {
			// Error other than file not existing (e.g., permission error)
			return err
		}
		// File doesn't exist or we're forcing overwrite, proceed to create it

		if (i+1)%10 == 0 {
			fmt.Printf("Processing package %d of %d: %s/%s:%s\n", i+1, len(pkgs), pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version())
		}

		var license string
		if len(pkg.Licenses()) > 0 {
			license = pkg.Licenses()[0]
		}

		description := pkg.Description()
		if description == "" {
			description = "Description not available"
		}

		pkgMetadata := PackageMetadata{
			Version: pkg.Version(),
			Name:    pkg.ArtifactName(),
			ContainersImages: []*ContainerImage{
				{
					Name:  pkg.ImageName(),
					Image: pkg.DigestRef(),
				},
			},
			Description: description,
			DisplayName: pkg.ArtifactName(),
			HomeURL:     pkg.Homepage(),
		}

		// Only set CreatedAt if it's not the zero value
		if !pkg.CreatedAt().IsZero() {
			pkgMetadata.CreatedAt = pkg.CreatedAt().Format(time.RFC3339)
		}

		// Only set License if it's not empty
		if license != "" {
			pkgMetadata.License = license
		}

		if err := os.MkdirAll(filepath.Dir(artifacthubPkgYml), 0755); err != nil {
			return err
		}

		// First, marshal without digest to calculate the digest
		packageYmlWithoutDigest, err := yaml.Marshal(pkgMetadata)
		if err != nil {
			return err
		}

		// Calculate SHA256 digest of the YAML content
		hash := sha256.Sum256(packageYmlWithoutDigest)
		digest := hex.EncodeToString(hash[:])

		// Set the digest in the metadata
		pkgMetadata.Digest = digest

		// Marshal again with the digest included
		packageYml, err := yaml.Marshal(pkgMetadata)
		if err != nil {
			return err
		}
		if err := os.WriteFile(artifacthubPkgYml, packageYml, 0644); err != nil {
			return err
		}

		// Extract and save buildpack.toml and package.toml from the image
		filesToExtract := make(map[string]string)
		filesToExtract["buildpack.toml"] = buildpackToml
		filesToExtract["package.toml"] = packageToml

		extractErrors := w.imageExtractor.ExtractFiles(pkg.DigestRef(), filesToExtract)
		for filename, err := range extractErrors {
			fmt.Printf("Warning: failed to extract %s for %s/%s:%s: %v\n",
				filename, pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version(), err)
		}
	}

	fmt.Printf("Wrote %d artifacthub-pkg.yml files to %s\n", len(pkgs), w.outputDir)

	return nil
}

func NewFilesystemOutputWriter(outputDir string) *FilesystemOutputWriter {
	return &FilesystemOutputWriter{
		outputDir:      outputDir,
		imageExtractor: NewImageExtractor(),
	}
}

type ImageOutputWriter struct {
	imageRef string
}

func (w *ImageOutputWriter) Write(pkgs []Package, force bool) error {
	return fmt.Errorf("not implemented")
}

func NewImageOutputWriter(imageRef string) *ImageOutputWriter {
	return &ImageOutputWriter{imageRef: imageRef}
}
