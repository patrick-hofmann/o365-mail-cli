package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourname/o365-mail-cli/internal/auth"
	"github.com/yourname/o365-mail-cli/internal/config"
)

var logoutAll bool

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  "Commands for login, logout, and OAuth2 authentication status.",
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Office 365",
	Long: `Starts the OAuth2 Device Code Flow.

You will receive a code to enter in your browser at microsoft.com/devicelogin.
After successful authentication, a token is stored locally.`,
	RunE: runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout [email]",
	Short: "Logout and delete token",
	Long: `Logs out an account and deletes its token.

Without argument, logs out the active account.
With --all, logs out all accounts.

Examples:
  o365-mail-cli auth logout
  o365-mail-cli auth logout user@example.com
  o365-mail-cli auth logout --all`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogout,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current auth status",
	RunE:  runStatus,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all logged-in accounts",
	RunE:  runList,
}

var switchCmd = &cobra.Command{
	Use:   "switch <email>",
	Short: "Switch default account",
	Long: `Switches the default account.

The selected account is used as default when no --account flag
or O365_ACCOUNT environment variable is set.

Examples:
  o365-mail-cli auth switch user@example.com`,
	Args: cobra.ExactArgs(1),
	RunE: runSwitch,
}

func init() {
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "Logout all accounts")

	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(statusCmd)
	authCmd.AddCommand(listCmd)
	authCmd.AddCommand(switchCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Create OAuth client
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	// Start device code flow
	printInfo("Starting login...")

	deviceCode, resultChan, err := oauthClient.StartDeviceCodeFlow(ctx)
	if err != nil {
		return fmt.Errorf("failed to start device code flow: %w", err)
	}

	// Show instructions
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  To sign in, open this URL in your browser:                ║")
	fmt.Printf("║  %s%s║\n", deviceCode.VerificationURL, spaces(36-len(deviceCode.VerificationURL)))
	fmt.Println("║                                                            ║")
	fmt.Println("║  And enter this code:                                      ║")
	fmt.Printf("║                        %s                            ║\n", deviceCode.UserCode)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	printInfo("Waiting for browser login...")

	// Wait for result
	result := <-resultChan

	if result.Error != nil {
		return fmt.Errorf("authentication failed: %w", result.Error)
	}

	// Add account to accounts.yaml
	if err := config.AddAccount(result.Email); err != nil {
		printError(fmt.Errorf("failed to save account: %w", err))
	}

	// Set as current_account
	if err := config.SetCurrentAccount(result.Email); err != nil {
		printError(fmt.Errorf("failed to set current account: %w", err))
	}

	fmt.Println()
	printSuccess("Successfully logged in as %s", result.Email)
	printInfo("Token valid until: %s", result.ExpiresAt.Format(time.RFC1123))
	printInfo("Token saved in: %s/token.json", cfg.CacheDir)
	printInfo("Account set as active account.")

	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	// --all flag: Logout all accounts
	if logoutAll {
		if err := oauthClient.LogoutAll(ctx); err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}
		if err := config.RemoveAllAccounts(); err != nil {
			printError(fmt.Errorf("failed to remove accounts from config: %w", err))
		}
		if err := config.SetCurrentAccount(""); err != nil {
			printError(fmt.Errorf("failed to clear current account: %w", err))
		}
		printSuccess("All accounts logged out")
		printInfo("Local tokens deleted.")
		return nil
	}

	// Determine email: argument > active account
	var email string
	if len(args) > 0 {
		email = args[0]
	} else {
		email = getActiveAccount()
	}

	if email == "" {
		printInfo("No account to logout.")
		return nil
	}

	// Check if account exists
	status, _ := oauthClient.GetStatus(ctx, email)
	if status == nil || !status.LoggedIn {
		printInfo("Account %s is not logged in.", email)
		return nil
	}

	// Logout
	if err := oauthClient.Logout(ctx, email); err != nil {
		return fmt.Errorf("logout failed: %w", err)
	}

	// Remove from accounts.yaml
	if err := config.RemoveAccount(email); err != nil {
		printError(fmt.Errorf("failed to remove account from config: %w", err))
	}

	// If it was the current_account, switch to another
	if cfg.CurrentAccount == email {
		newCurrent := config.GetFirstAccount()
		if err := config.SetCurrentAccount(newCurrent); err != nil {
			printError(fmt.Errorf("failed to update current account: %w", err))
		}
		if newCurrent != "" {
			printInfo("Active account switched to: %s", newCurrent)
		}
	}

	printSuccess("Successfully logged out from %s", email)
	printInfo("Token deleted.")

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	statuses, err := oauthClient.GetAllStatuses(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	fmt.Println()
	fmt.Println("Auth Status")
	fmt.Println("───────────")

	if len(statuses) == 0 {
		printInfo("Status: Not logged in")
		printInfo("\nUse 'auth login' to sign in.")
		return nil
	}

	currentAccount := getActiveAccount()

	for _, status := range statuses {
		marker := "  "
		if status.Email == currentAccount {
			marker = "* "
		}

		if status.TokenExpired {
			fmt.Printf("%s%s (token expired)\n", marker, status.Email)
		} else {
			remaining := time.Until(status.ExpiresAt)
			var remainingStr string
			if remaining > time.Hour {
				remainingStr = fmt.Sprintf("%.0fh", remaining.Hours())
			} else {
				remainingStr = fmt.Sprintf("%.0fm", remaining.Minutes())
			}
			fmt.Printf("%s%s (valid, %s remaining)\n", marker, status.Email, remainingStr)
		}
	}

	fmt.Println()
	printInfo("* = active account")

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	accounts, err := config.LoadAccounts()
	if err != nil {
		return fmt.Errorf("failed to load accounts: %w", err)
	}

	if len(accounts) == 0 {
		printInfo("No accounts logged in.")
		printInfo("\nUse 'auth login' to sign in.")
		return nil
	}

	currentAccount := getActiveAccount()

	fmt.Println()
	fmt.Println("Logged-in Accounts")
	fmt.Println("──────────────────")

	for _, acc := range accounts {
		marker := "  "
		if acc.Email == currentAccount {
			marker = "* "
		}
		addedAt := acc.AddedAt.Format("2006-01-02 15:04")
		fmt.Printf("%s%s (added: %s)\n", marker, acc.Email, addedAt)
	}

	fmt.Println()
	printInfo("* = active account")
	printInfo("\nUse 'auth switch <email>' to change the active account.")

	return nil
}

func runSwitch(cmd *cobra.Command, args []string) error {
	email := args[0]

	// Check if account exists
	if !config.AccountExists(email) {
		return fmt.Errorf("account %s not found. Use 'auth list' to show all accounts", email)
	}

	// Set as current_account
	if err := config.SetCurrentAccount(email); err != nil {
		return fmt.Errorf("failed to set current account: %w", err)
	}

	printSuccess("Active account switched to: %s", email)

	return nil
}

// spaces returns n spaces
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	s := ""
	for i := 0; i < n; i++ {
		s += " "
	}
	return s
}
