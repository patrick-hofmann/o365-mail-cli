package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourname/o365-mail-cli/internal/config"
)

var (
	cfg         *config.Config
	cfgFile     string
	debug       bool
	accountFlag string
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "o365-mail-cli",
	Short: "Office 365 Email CLI with OAuth2",
	Long: `A cross-platform CLI tool for Office 365 email access.

Uses OAuth2 Device Code Flow for authentication -
no admin approval or API keys required.

Examples:
  # Login
  o365-mail-cli auth login

  # List emails
  o365-mail-cli mail list

  # Send email
  o365-mail-cli mail send --to recipient@example.com --subject "Test" --body "Hello!"`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if debug {
			cfg.Debug = true
		}

		return nil
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file (default: ~/.o365-mail-cli/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")
	rootCmd.PersistentFlags().StringVar(&accountFlag, "account", "", "Account to use (email address)")

	// Add subcommands
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(mailCmd)
	rootCmd.AddCommand(foldersCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

// getActiveAccount returns the active account
// Priority: 1. --account flag, 2. O365_ACCOUNT env, 3. current_account from config
func getActiveAccount() string {
	// 1. --account flag
	if accountFlag != "" {
		return accountFlag
	}

	// 2. O365_ACCOUNT environment variable
	if envAccount := os.Getenv("O365_ACCOUNT"); envAccount != "" {
		return envAccount
	}

	// 3. current_account from config
	if cfg != nil && cfg.CurrentAccount != "" {
		return cfg.CurrentAccount
	}

	// Fallback: First account from accounts.yaml
	return config.GetFirstAccount()
}

// versionCmd shows the version
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("o365-mail-cli v1.2.0")
	},
}

// debugLog prints debug messages when enabled
func debugLog(format string, args ...interface{}) {
	if cfg != nil && cfg.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// printError prints a formatted error
func printError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}

// printSuccess prints a success message
func printSuccess(format string, args ...interface{}) {
	fmt.Printf("✓ "+format+"\n", args...)
}

// printInfo prints an info message
func printInfo(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}
