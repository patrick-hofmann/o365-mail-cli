package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourname/o365-mail-cli/internal/auth"
	"github.com/yourname/o365-mail-cli/internal/mail"
)

var foldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "Manage folders",
	Long:  "Commands for listing, creating, and deleting IMAP folders.",
}

var foldersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all folders",
	RunE:  runFoldersList,
}

var foldersCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create new folder",
	Long: `Creates a new IMAP folder.

For subfolders, use '/' as delimiter.

Examples:
  o365-mail-cli folders create "Archive"
  o365-mail-cli folders create "Archive/2024"`,
	Args: cobra.ExactArgs(1),
	RunE: runFoldersCreate,
}

var foldersDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete folder",
	Long: `Deletes an IMAP folder.

Warning: All emails in the folder will be deleted!

Examples:
  o365-mail-cli folders delete "Old Folder"`,
	Args: cobra.ExactArgs(1),
	RunE: runFoldersDelete,
}

func init() {
	foldersCmd.AddCommand(foldersListCmd)
	foldersCmd.AddCommand(foldersCreateCmd)
	foldersCmd.AddCommand(foldersDeleteCmd)
}

func runFoldersList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get active account
	email := getActiveAccount()
	if email == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, email)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, email, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// Fetch folders
	folders, err := imapClient.ListFolders()
	if err != nil {
		return err
	}

	fmt.Println("\nAvailable Folders:")
	fmt.Println("──────────────────")

	for _, folder := range folders {
		// Indentation based on hierarchy
		depth := strings.Count(folder.Name, folder.Delimiter)
		indent := strings.Repeat("  ", depth)

		// Icon based on attributes
		icon := "📁"
		for _, attr := range folder.Attributes {
			switch attr {
			case "\\Sent":
				icon = "📤"
			case "\\Drafts":
				icon = "📝"
			case "\\Trash":
				icon = "🗑️"
			case "\\Junk":
				icon = "⚠️"
			case "\\Archive":
				icon = "📦"
			}
		}

		// Show only last part of name for better readability
		displayName := folder.Name
		if folder.Delimiter != "" {
			parts := strings.Split(folder.Name, folder.Delimiter)
			displayName = parts[len(parts)-1]
		}

		fmt.Printf("%s%s %s\n", indent, icon, displayName)
	}

	fmt.Printf("\n%d folders found\n", len(folders))

	return nil
}

func runFoldersCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	folderName := args[0]

	// Get active account
	email := getActiveAccount()
	if email == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, email)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, email, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// Create folder
	if err := imapClient.CreateFolder(folderName); err != nil {
		return err
	}

	printSuccess("Folder '%s' created", folderName)

	return nil
}

func runFoldersDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	folderName := args[0]

	// Get active account
	email := getActiveAccount()
	if email == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Confirmation
	fmt.Printf("Delete folder '%s'? All emails will be lost! [y/N]: ", folderName)
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		printInfo("Cancelled.")
		return nil
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, email)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, email, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// Delete folder
	if err := imapClient.DeleteFolder(folderName); err != nil {
		return err
	}

	printSuccess("Folder '%s' deleted", folderName)

	return nil
}
