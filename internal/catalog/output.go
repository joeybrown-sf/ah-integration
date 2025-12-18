package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

type OutputWriter interface {
	Write(pkgs []Package, force bool) error
}

type FilesystemOutputWriter struct {
	outputDir string
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
	CreatedAt               string            `yaml:"createdAt"`
	Description             string            `yaml:"description"`
	LogoPath                string            `yaml:"logoPath"`
	LogoURL                 string            `yaml:"logoURL"`
	Digest                  string            `yaml:"digest"`
	License                 string            `yaml:"license"`
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
	for _, pkg := range pkgs {

		artifacthubPkgYml := filepath.Join(w.outputDir, pkg.ArtifactRepository(), pkg.ArtifactName(), pkg.Version(), "artifacthub-pkg.yml")

		stat, err := os.Stat(artifacthubPkgYml)
		if err == nil {
			// File exists, check if we should skip it
			if !force && stat.Size() > 0 {
				continue
			}
		} else if !os.IsNotExist(err) {
			// Error other than file not existing (e.g., permission error)
			return err
		}
		// File doesn't exist or we're forcing overwrite, proceed to create it

		pkgMetadata := PackageMetadata{
			Version: pkg.Version(),
			Name:    pkg.ArtifactName(),
			ContainersImages: []*ContainerImage{
				{
					Name:        pkg.ImageName(),
					Image:       pkg.VersionTagRef(),
					Whitelisted: true,
				},
			},
			Digest: pkg.ImageDigest(),
		}

		if err := os.MkdirAll(filepath.Dir(artifacthubPkgYml), 0755); err != nil {
			return err
		}

		packageYml, err := yaml.Marshal(pkgMetadata)
		if err != nil {
			return err
		}
		if err := os.WriteFile(artifacthubPkgYml, packageYml, 0644); err != nil {
			return err

		}
	}

	fmt.Printf("Wrote %d artifacthub-pkg.yml files to %s\n", len(pkgs), w.outputDir)

	return nil
}

func NewFilesystemOutputWriter(outputDir string) *FilesystemOutputWriter {
	return &FilesystemOutputWriter{outputDir: outputDir}
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
