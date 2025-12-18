package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joeybrown-sf/ah-integration/internal/catalog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate artifacthub-pkg.yml files",
	Long:  `Generate artifacthub-pkg.yml files using different output methods (filesystem or image)`,
}

var generateFilesystemCmd = &cobra.Command{
	Use:   "filesystem",
	Short: "Write artifacthub-pkg.yml files to filesystem (for gitops)",
	Long:  `Walk through directories (excluding hidden ones) and create artifacthub-pkg.yml files in .catalog directories`,
	Run: runGenerate(func(cmd *cobra.Command, root string) catalog.OutputWriter {
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if outputDir != "" {
			return catalog.NewFilesystemOutputWriter(outputDir)
		}
		return catalog.NewFilesystemOutputWriter(filepath.Join(root, "catalog"))
	}),
}

func runGenerate(getOutputWriter func(cmd *cobra.Command, _ string) catalog.OutputWriter) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		root := viper.GetString("root")
		if root == "" {
			tempDir, err := os.MkdirTemp("", "ah-integration")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Using root: %s\n", tempDir)
			root = tempDir
		}

		force := viper.GetBool("force")
		migrationThresholdMonths, _ := cmd.Flags().GetInt("migration-threshold-mo")

		var namespaces []string
		if cmd.Flags().Changed("namespaces") {
			namespaces, _ = cmd.Flags().GetStringSlice("namespaces")
		} else {
			namespaces = viper.GetStringSlice("namespaces")
		}

		var ignoredRegistries []string
		if cmd.Flags().Changed("ignored-registries") {
			ignoredRegistries, _ = cmd.Flags().GetStringSlice("ignored-registries")
		} else {
			ignoredRegistries = viper.GetStringSlice("ignored_registries")
		}

		migrationThreshold := time.Duration(migrationThresholdMonths) * 30 * 24 * time.Hour

		outputWriter := getOutputWriter(cmd, root)
		generator := catalog.NewGenerator(migrationThreshold, root, outputWriter)
		err := generator.GenerateCatalogFiles(force, namespaces, ignoredRegistries)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

var generateImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Create image representation of artifacthub-pkg.yml files",
	Long:  `Generate artifacthub-pkg.yml files and create an image representation`,
	Run: runGenerate(func(cmd *cobra.Command, _ string) catalog.OutputWriter {
		imageRef, _ := cmd.Flags().GetString("image")
		if imageRef == "" {
			fmt.Fprintf(os.Stderr, "Error: --image flag is required for image subcommand\n")
			os.Exit(1)
		}

		outputWriter := catalog.NewImageOutputWriter(imageRef)
		return outputWriter
	}),
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(generateFilesystemCmd)
	generateCmd.AddCommand(generateImageCmd)

	// Common flags for both subcommands
	generateCmd.PersistentFlags().BoolP("force", "f", false, "Overwrite existing artifacthub-pkg.yml files")
	generateCmd.PersistentFlags().StringSlice("namespaces", []string{}, "Filter packages by namespace(s). Can be specified multiple times or as comma-separated values")
	generateCmd.PersistentFlags().StringSlice("ignored-registries", []string{}, "Ignore packages from registry(ies). Can be specified multiple times or as comma-separated values")
	generateCmd.PersistentFlags().Int("migration-threshold-mo", 24, "Threshold for migration in months")

	// Filesystem-specific flags
	generateFilesystemCmd.Flags().String("output-dir", "", "Output directory for artifacthub-pkg.yml files (default: root directory)")

	// Image-specific flags
	generateImageCmd.Flags().String("image", "", "Image reference for the output image (required)")

	viper.BindPFlag("force", generateCmd.PersistentFlags().Lookup("force"))
	viper.BindPFlag("namespaces", generateCmd.PersistentFlags().Lookup("namespaces"))
	viper.BindPFlag("ignored_registries", generateCmd.PersistentFlags().Lookup("ignored-registries"))
}
