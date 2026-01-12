package catalog

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageExtractor handles exporting container images and extracting files from them
type ImageExtractor struct {
}

// NewImageExtractor creates a new ImageExtractor
func NewImageExtractor() *ImageExtractor {
	return &ImageExtractor{}
}

// ExtractFiles pulls the image and extracts multiple files from it
// files is a map of filename -> outputPath
// Returns a map of filename -> error for any files that failed to extract
func (e *ImageExtractor) ExtractFiles(imageRef string, files map[string]string) map[string]error {
	errors := make(map[string]error)

	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		// If we can't parse the reference, all files fail
		for filename := range files {
			errors[filename] = fmt.Errorf("failed to parse image reference %s: %w", imageRef, err)
		}
		return errors
	}

	// Pull the image
	img, err := remote.Image(ref)
	if err != nil {
		// If we can't pull the image, all files fail
		for filename := range files {
			errors[filename] = fmt.Errorf("failed to pull image %s: %w", imageRef, err)
		}
		return errors
	}

	// Extract all requested files from the image layers
	for filename, outputPath := range files {
		if err := e.extractFileFromImage(img, filename, outputPath); err != nil {
			errors[filename] = fmt.Errorf("failed to extract %s from image: %w", filename, err)
		}
	}

	return errors
}

// ExtractBuildpackTOML pulls the image and extracts buildpack.toml from it
// Deprecated: Use ExtractFiles instead for better efficiency
func (e *ImageExtractor) ExtractBuildpackTOML(imageRef string, outputPath string) error {
	errors := e.ExtractFiles(imageRef, map[string]string{
		"buildpack.toml": outputPath,
	})
	if err, ok := errors["buildpack.toml"]; ok {
		return err
	}
	return nil
}

// extractFileFromImage extracts a specific file from an image's filesystem
func (e *ImageExtractor) extractFileFromImage(img v1.Image, targetFile, outputPath string) error {
	// Get the layers
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get image layers: %w", err)
	}

	// Search through layers in reverse order (top layer first)
	// This ensures we get the most recent version of the file
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]

		// Get the uncompressed layer content (tar format)
		uncompressed, err := layer.Uncompressed()
		if err != nil {
			// Skip this layer if we can't read it
			continue
		}

		// Create tar reader from the layer
		tarReader := tar.NewReader(uncompressed)

		// Search for the target file in this layer
		found, err := e.searchAndExtractFile(tarReader, targetFile, outputPath)
		uncompressed.Close()
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}

	return fmt.Errorf("file not found in image")
}

// searchAndExtractFile searches for a file in a tar reader and extracts it if found
func (e *ImageExtractor) searchAndExtractFile(tarReader *tar.Reader, targetFile, outputPath string) (bool, error) {
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Check if this is the file we're looking for
		// buildpack.toml might be at the root or in a subdirectory
		// We check if the path ends with the target filename
		if header.Name == targetFile || strings.HasSuffix(header.Name, "/"+targetFile) {
			// Ensure output directory exists
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				return false, fmt.Errorf("failed to create output directory: %w", err)
			}

			// Create the output file
			outFile, err := os.Create(outputPath)
			if err != nil {
				return false, fmt.Errorf("failed to create output file: %w", err)
			}
			defer outFile.Close()

			// Copy the file contents
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return false, fmt.Errorf("failed to copy file contents: %w", err)
			}

			return true, nil
		}
	}

	return false, nil
}
