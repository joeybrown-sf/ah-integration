package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/joeybrown-sf/ah-integration/internal/catalog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate artifacthub-pkg.yml files in .catalog directories",
	Long:  `Walk through directories (excluding hidden ones) and create artifacthub-pkg.yml files`,
	Run: func(cmd *cobra.Command, args []string) {
		root := viper.GetString("root")
		if root == "" {
			tempDir, err := os.MkdirTemp("", "registry-index")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Using root: %s\n", tempDir)
			root = tempDir
		}

		var migrationThresholdMonths *int
		force := viper.GetBool("force")
		migrationThresholdMonths = cmd.Flags().Int("migration-threshold-mo", 24, "Threshold for migration in months")

		// Get namespaces from flag or config
		var namespaces []string
		if cmd.Flags().Changed("namespaces") {
			namespaces, _ = cmd.Flags().GetStringSlice("namespaces")
		} else {
			namespaces = viper.GetStringSlice("namespaces")
		}

		// Get ignored_registries from flag or config
		var ignoredRegistries []string
		if cmd.Flags().Changed("ignored-registries") {
			ignoredRegistries, _ = cmd.Flags().GetStringSlice("ignored-registries")
		} else {
			ignoredRegistries = viper.GetStringSlice("ignored_registries")
		}

		migrationThreshold := time.Duration(*migrationThresholdMonths) * 30 * 24 * time.Hour
		generator := catalog.NewGenerator(migrationThreshold, root)

		err := generator.GenerateCatalogFiles(force, namespaces, ignoredRegistries)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolP("force", "f", false, "Overwrite existing artifacthub-pkg.yml files")
	generateCmd.Flags().StringSlice("namespaces", []string{}, "Filter packages by namespace(s). Can be specified multiple times or as comma-separated values")
	generateCmd.Flags().StringSlice("ignored-registries", []string{}, "Ignore packages from registry(ies). Can be specified multiple times or as comma-separated values")
	viper.BindPFlag("force", generateCmd.Flags().Lookup("force"))
	viper.BindPFlag("namespaces", generateCmd.Flags().Lookup("namespaces"))
	viper.BindPFlag("ignored_registries", generateCmd.Flags().Lookup("ignored-registries"))
}
