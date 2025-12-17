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
	Yanked() bool
	ArtifactRepository() string
	ArtifactName() string
	WillMigrate(registryClient *BuildpackRegistryClient) bool
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
}

func (a *artifact) ImageRepository() string    { return a.imageRepository }
func (a *artifact) ImageName() string          { return a.imageName }
func (a *artifact) ImageDigest() string        { return a.imageDigest }
func (a *artifact) Version() string            { return a.version }
func (a *artifact) Yanked() bool               { return a.yanked }
func (a *artifact) ArtifactRepository() string { return a.artifactRepository }
func (a *artifact) ArtifactName() string       { return a.artifactName }

type DockerhubPackage struct{ artifact }

func (*DockerhubPackage) ImageRegistry() string { return "index.docker.io" }
func (p *DockerhubPackage) WillMigrate(registryClient *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	metadata, err := registryClient.GetBuildpackRegistry(p)
	if err != nil {
		fmt.Printf("failed to get buildpack registry for %s/%s: %v\n", p.ImageRepository(), p.ImageName(), err)
		return false
	}

	return metadata.IsRecent(p.migrationThreshold)
}

type ECRPackage struct{ artifact }

func (*ECRPackage) ImageRegistry() string { return "public.ecr.aws" }

// There were a handful of packages that used ECR, but none will be migrated.
func (p *ECRPackage) WillMigrate(_ *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	// Spot checked images--haven't been updated in almost 2 years
	if p.ArtifactRepository() == "initializ-buildpacks" ||
		p.ArtifactRepository() == "initializ-buildpack" ||
		p.ArtifactRepository() == "naveeninitializ" ||
		p.ArtifactRepository() == "vivek-buildpacks" {
		return false
	}

	// Spot checked ecr--couldn't find anything
	if p.ArtifactRepository() == "malax" || p.ArtifactRepository() == "nv" {
		return false
	}

	return true
}

type GHCRPackage struct{ artifact }

func (*GHCRPackage) ImageRegistry() string { return "ghcr.io" }
func (p *GHCRPackage) WillMigrate(registryClient *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	metadata, err := registryClient.GetBuildpackRegistry(p)
	if err != nil {
		fmt.Printf("failed to get buildpack registry for %s/%s: %v\n", p.ImageRepository(), p.ImageName(), err)
		return false
	}

	return metadata.IsRecent(p.migrationThreshold)
}

type GCRPackage struct{ artifact }

func (*GCRPackage) ImageRegistry() string { return "gcr.io" }
func (p *GCRPackage) WillMigrate(registryClient *BuildpackRegistryClient) bool {
	if p.Yanked() {
		return false
	}

	metadata, err := registryClient.GetBuildpackRegistry(p)
	if err != nil {
		fmt.Printf("failed to get buildpack registry for %s/%s: %v\n", p.ImageRepository(), p.ImageName(), err)
		return false
	}

	return metadata.IsRecent(p.migrationThreshold)
}

type FuturehaxPackage struct{ artifact }

func (*FuturehaxPackage) ImageRegistry() string { return "registry.futurehax.com" }

// 0.0.1 -> registry.futurehax.com/futurehax/androidbuildpack@sha256:41983061e83937e1c4c01b2a388c37f6da5075dd6e59dbb0a2e077e54ab0f5bd
// 0.0.2 -> registry.futurehax.com/futurehax/androidbuildpack@sha256:96df7c7a960892061cb1d7b45957a852c04f05e84934e9fb966565d8fd4d47a1
// 0.0.3 -> registry.futurehax.com/futurehax/androidbuildpack@sha256:874ddf3f9e03d573375ffdc8ec51a8e06ccf730f7e1b17aee37621451c5bbe73
// I got redirected and DENIED: access forbidden. So I'm not going to migrate it.
func (p *FuturehaxPackage) WillMigrate(_ *BuildpackRegistryClient) bool { return false }

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
