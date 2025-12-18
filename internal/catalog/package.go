package catalog

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Package interface {
	ImageRegistry() string
	ImageRepository() string
	ImageName() string
	ImageDigest() string
	Version() string
	Description() string
	Yanked() bool
	ArtifactRepository() string
	ArtifactName() string
	CreatedAt() time.Time
	WillMigrate(registryClient *BuildpackRegistryClient) bool
	DigestRef() string
	VersionTagRef() string
	Licenses() []string
	Homepage() string
}

type artifact struct {
	imageRepository    string
	imageName          string
	imageDigest        string
	version            string
	yanked             bool
	artifactRepository string
	artifactName       string
	migrationThreshold time.Duration
	createdAt          time.Time
	description        string
	licenses           []string
	homepage           string
}

func (a *artifact) ImageRepository() string    { return a.imageRepository }
func (a *artifact) ImageName() string          { return a.imageName }
func (a *artifact) ImageDigest() string        { return a.imageDigest }
func (a *artifact) Version() string            { return a.version }
func (a *artifact) Yanked() bool               { return a.yanked }
func (a *artifact) ArtifactRepository() string { return a.artifactRepository }
func (a *artifact) ArtifactName() string       { return a.artifactName }
func (a *artifact) CreatedAt() time.Time       { return a.createdAt }
func (a *artifact) SetCreatedAt(t time.Time)    { a.createdAt = t }
func (a *artifact) Description() string         { return a.description }
func (a *artifact) Licenses() []string          { return a.licenses }
func (a *artifact) Homepage() string            { return a.homepage }

func (a *artifact) GetBuildpackRegistryMetadata(registryClient *BuildpackRegistryClient, force bool) (registryMetadata, bool) {
	metadata, err := registryClient.GetBuildpackRegistryMetadata(a, force)
	if err != nil {
		fmt.Printf("failed to get buildpack registry for %s/%s: %v\n", a.ImageRepository(), a.ImageName(), err)
		return registryMetadata{}, false
	}
	return *metadata, true
}

type DockerhubPackage struct{ artifact }

func (*DockerhubPackage) ImageRegistry() string { return "index.docker.io" }
func (p *DockerhubPackage) WillMigrate(registryClient *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	metadata, exists := p.GetBuildpackRegistryMetadata(registryClient, false)
	return exists && metadata.IsRecent(p.migrationThreshold)
}

func (p *DockerhubPackage) DigestRef() string {
	return fmt.Sprintf("%s/%s/%s@%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.ImageDigest())
}

func (p *DockerhubPackage) VersionTagRef() string {
	return fmt.Sprintf("%s/%s/%s:%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.Version())
}

type ECRPackage struct{ artifact }

func (*ECRPackage) ImageRegistry() string { return "public.ecr.aws" }

func (p *ECRPackage) WillMigrate(_ *BuildpackRegistryClient) bool { return false }

func (p *ECRPackage) DigestRef() string {
	return fmt.Sprintf("%s/%s/%s@%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.ImageDigest())
}

func (p *ECRPackage) VersionTagRef() string {
	return fmt.Sprintf("%s/%s/%s:%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.Version())
}

type GHCRPackage struct{ artifact }

func (*GHCRPackage) ImageRegistry() string { return "ghcr.io" }
func (p *GHCRPackage) WillMigrate(registryClient *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	metadata, exists := p.GetBuildpackRegistryMetadata(registryClient, false)
	return exists && metadata.IsRecent(p.migrationThreshold)
}

func (p *GHCRPackage) DigestRef() string {
	return fmt.Sprintf("%s/%s/%s@%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.ImageDigest())
}

func (p *GHCRPackage) VersionTagRef() string {
	return fmt.Sprintf("%s/%s/%s:%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.Version())
}

type GCRPackage struct{ artifact }

func (*GCRPackage) ImageRegistry() string                         { return "gcr.io" }
func (p *GCRPackage) WillMigrate(_ *BuildpackRegistryClient) bool { return false }

func (p *GCRPackage) DigestRef() string {
	return fmt.Sprintf("%s/%s/%s@%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.ImageDigest())
}

func (p *GCRPackage) VersionTagRef() string {
	return fmt.Sprintf("%s/%s/%s:%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.Version())
}

type FuturehaxPackage struct{ artifact }

func (*FuturehaxPackage) ImageRegistry() string                         { return "registry.futurehax.com" }
func (p *FuturehaxPackage) WillMigrate(_ *BuildpackRegistryClient) bool { return false }

func (p *FuturehaxPackage) DigestRef() string {
	return fmt.Sprintf("%s/%s/%s@%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.ImageDigest())
}

func (p *FuturehaxPackage) VersionTagRef() string {
	return fmt.Sprintf("%s/%s/%s:%s", p.ImageRegistry(), p.ImageRepository(), p.ImageName(), p.Version())
}

func NewPackageFactory(httpClient *http.Client, migrationThreshold time.Duration) *PackageFactory {
	return &PackageFactory{
		httpClient:         httpClient,
		migrationThreshold: migrationThreshold,
	}
}

type PackageFactory struct {
	httpClient         *http.Client
	migrationThreshold time.Duration
}

func (v *PackageFactory) CreatePackage(artifactRepository string, artifactName string, rawPkg rawPackage) (Package, error) {
	addrParts := strings.Split(rawPkg.Addr, "@")
	if len(addrParts) != 2 {
		return nil, fmt.Errorf("invalid addr: %s", rawPkg.Addr)
	}

	// len is going to be 3 or 4
	repoParts := strings.Split(addrParts[0], "/")

	registryHost := repoParts[0]

	artifact := artifact{
		artifactName:       artifactName,
		artifactRepository: artifactRepository,
		imageDigest:        addrParts[1],
		imageName:          strings.Join(repoParts[2:], "/"),
		imageRepository:    repoParts[1],
		version:            rawPkg.Version,
		yanked:             rawPkg.Yanked,
		migrationThreshold: v.migrationThreshold,
		createdAt:          rawPkg.CreatedAt,
		description:        rawPkg.Description,
		licenses:           rawPkg.Licenses,
		homepage:           rawPkg.Homepage,
	}

	switch registryHost {
	case "docker.io", "index.docker.io":
		return &DockerhubPackage{artifact}, nil
	case "ghcr.io":
		return &GHCRPackage{artifact}, nil
	case "gcr.io":
		return &GCRPackage{artifact}, nil
	case "registry.futurehax.com":
		return &FuturehaxPackage{artifact}, nil
	case "public.ecr.aws":
		return &ECRPackage{artifact}, nil
	}

	return nil, fmt.Errorf("unsupported registry: %s", registryHost)
}
