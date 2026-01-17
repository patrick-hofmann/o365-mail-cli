package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourname/o365-mail-cli/internal/auth"
	"github.com/yourname/o365-mail-cli/internal/mail"
)

var draftsCmd = &cobra.Command{
	Use:   "drafts",
	Short: "Manage email drafts",
	Long:  "Commands for managing email drafts.",
}

// Draft create command
var (
	draftTo       []string
	draftCc       []string
	draftSubject  string
	draftBody     string
	draftBodyFile string
	draftHTML     bool
)

var draftCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a draft email",
	Long: `Creates a draft email and saves it to the Drafts folder.

Examples:
  o365-mail-cli mail drafts create --to user@example.com --subject "Test" --body "Hello!"
  o365-mail-cli mail drafts create --to user@example.com --subject "Report" --body-file draft.txt`,
	RunE: runDraftCreate,
}

// Draft list command
var draftListJSON bool

var draftListCmd = &cobra.Command{
	Use:   "list",
	Short: "List drafts",
	Long: `Lists all draft emails.

Examples:
  o365-mail-cli mail drafts list
  o365-mail-cli mail drafts list --json`,
	RunE: runDraftList,
}

// Draft send command
var draftSendCmd = &cobra.Command{
	Use:   "send [uid]",
	Short: "Send a draft",
	Long: `Sends a draft email and removes it from the Drafts folder.

Examples:
  o365-mail-cli mail drafts send 12345`,
	Args: cobra.ExactArgs(1),
	RunE: runDraftSend,
}

// Draft delete command
var draftDeleteCmd = &cobra.Command{
	Use:   "delete [uid]",
	Short: "Delete a draft",
	Long: `Deletes a draft email from the Drafts folder.

Examples:
  o365-mail-cli mail drafts delete 12345`,
	Args: cobra.ExactArgs(1),
	RunE: runDraftDelete,
}

func init() {
	// Draft create flags
	draftCreateCmd.Flags().StringArrayVar(&draftTo, "to", nil, "Recipients")
	draftCreateCmd.Flags().StringArrayVar(&draftCc, "cc", nil, "CC recipients")
	draftCreateCmd.Flags().StringVar(&draftSubject, "subject", "", "Subject")
	draftCreateCmd.Flags().StringVar(&draftBody, "body", "", "Message body")
	draftCreateCmd.Flags().StringVar(&draftBodyFile, "body-file", "", "Read body from file")
	draftCreateCmd.Flags().BoolVar(&draftHTML, "html", false, "Body is HTML")

	draftCreateCmd.MarkFlagRequired("to")
	draftCreateCmd.MarkFlagRequired("subject")

	// Draft list flags
	draftListCmd.Flags().BoolVar(&draftListJSON, "json", false, "Output as JSON")

	// Add subcommands
	draftsCmd.AddCommand(draftCreateCmd)
	draftsCmd.AddCommand(draftListCmd)
	draftsCmd.AddCommand(draftSendCmd)
	draftsCmd.AddCommand(draftDeleteCmd)

	// Add drafts command to mail
	mailCmd.AddCommand(draftsCmd)
}

func runDraftCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validation
	if len(draftTo) == 0 {
		return fmt.Errorf("at least one recipient (--to) required")
	}

	// Body from file or direct
	body := draftBody
	if draftBodyFile != "" {
		content, err := os.ReadFile(draftBodyFile)
		if err != nil {
			return fmt.Errorf("could not read body file: %w", err)
		}
		body = string(content)
	}

	if body == "" {
		return fmt.Errorf("message body required (--body or --body-file)")
	}

	// Get active account
	account := getActiveAccount()
	if account == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// Create draft
	draft := mail.DraftEmail{
		From:    account,
		To:      draftTo,
		Cc:      draftCc,
		Subject: draftSubject,
		Body:    body,
		HTML:    draftHTML,
	}

	if err := imapClient.SaveDraft(draft); err != nil {
		return err
	}

	printSuccess("Draft saved to Drafts folder")
	return nil
}

func runDraftList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get active account
	account := getActiveAccount()
	if account == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// List drafts
	drafts, err := imapClient.ListDrafts(50)
	if err != nil {
		return err
	}

	// Output
	if draftListJSON {
		return outputJSON(drafts)
	}

	if len(drafts) == 0 {
		printInfo("No drafts found.")
		return nil
	}

	fmt.Printf("\n%-8s %-20s %-30s %s\n", "UID", "Date", "To", "Subject")
	fmt.Println(strings.Repeat("-", 100))

	for _, draft := range drafts {
		to := ""
		if len(draft.To) > 0 {
			to = truncate(draft.To[0], 28)
		}
		subject := truncate(draft.Subject, 35)
		date := draft.Date.Local().Format("2006-01-02 15:04")

		fmt.Printf("  %-7d %-20s %-30s %s\n", draft.UID, date, to, subject)
	}

	fmt.Printf("\n%d draft(s) found\n", len(drafts))

	return nil
}

func runDraftSend(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse UID
	var uid uint32
	if _, err := fmt.Sscanf(args[0], "%d", &uid); err != nil {
		return fmt.Errorf("invalid UID: %s", args[0])
	}

	// Get active account
	account := getActiveAccount()
	if account == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client - fetch draft
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}

	draft, err := imapClient.GetEmail("Drafts", uid)
	if err != nil {
		imapClient.Close()
		return fmt.Errorf("failed to fetch draft: %w", err)
	}

	// SMTP Client
	smtpClient := mail.NewSMTPClient(account, cfg.SMTPServer, cfg.SMTPPort)

	debugLog("Sending draft via %s:%d", cfg.SMTPServer, cfg.SMTPPort)

	// Send
	opts := mail.SendOptions{
		To:      draft.To,
		Subject: draft.Subject,
		Body:    draft.Body,
	}

	if err := smtpClient.Send(accessToken, opts); err != nil {
		imapClient.Close()
		return fmt.Errorf("send failed: %w", err)
	}

	// Delete draft after successful send
	if err := imapClient.DeleteDraft(uid); err != nil {
		imapClient.Close()
		return fmt.Errorf("sent but failed to delete draft: %w", err)
	}
	imapClient.Close()

	printSuccess("Draft %d sent to %s", uid, strings.Join(draft.To, ", "))
	return nil
}

func runDraftDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse UID
	var uid uint32
	if _, err := fmt.Sscanf(args[0], "%d", &uid); err != nil {
		return fmt.Errorf("invalid UID: %s", args[0])
	}

	// Get active account
	account := getActiveAccount()
	if account == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return err
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("not logged in: %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	// Delete draft
	if err := imapClient.DeleteDraft(uid); err != nil {
		return err
	}

	printSuccess("Draft %d deleted", uid)
	return nil
}
