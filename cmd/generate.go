package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/joeybrown-sf/ah-integration/internal/catalog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// generateCmd represents the generate command
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

		migrationThreshold := time.Duration(*migrationThresholdMonths) * 30 * 24 * time.Hour
		generator := catalog.NewGenerator(migrationThreshold, root, force)

		err := generator.GenerateCatalogFiles()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolP("force", "f", false, "Overwrite existing artifacthub-pkg.yml files")
	viper.BindPFlag("force", generateCmd.Flags().Lookup("force"))
}
