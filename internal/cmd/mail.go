package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourname/o365-mail-cli/internal/auth"
	"github.com/yourname/o365-mail-cli/internal/mail"
)

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Manage emails",
	Long:  "Commands for reading and sending emails.",
}

// List Command
var (
	listFolder     string
	listLimit      uint32
	listUnreadOnly bool
	listJSON       bool
)

var mailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List emails",
	Long: `Lists emails from a folder.

Examples:
  o365-mail-cli mail list
  o365-mail-cli mail list --folder "Sent Items" --limit 20
  o365-mail-cli mail list --unread
  o365-mail-cli mail list --json`,
	RunE: runMailList,
}

// Read Command
var readFolder string

var readCmd = &cobra.Command{
	Use:   "read [uid]",
	Short: "Read email",
	Long: `Shows the content of an email.

Find the UID in the output of 'mail list'.

Examples:
  o365-mail-cli mail read 12345
  o365-mail-cli mail read 12345 --folder "Sent Items"`,
	Args: cobra.ExactArgs(1),
	RunE: runRead,
}

// Send Command
var (
	sendTo       []string
	sendCc       []string
	sendBcc      []string
	sendSubject  string
	sendBody     string
	sendBodyFile string
	sendHTML     bool
	sendAttach   []string
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send email",
	Long: `Sends an email.

Examples:
  o365-mail-cli mail send --to user@example.com --subject "Test" --body "Hello!"
  o365-mail-cli mail send --to user@example.com --subject "Report" --body-file report.txt
  o365-mail-cli mail send --to user@example.com --cc boss@example.com --subject "Info" --body "Text" --attach file.pdf`,
	RunE: runSend,
}

func init() {
	// List flags
	mailListCmd.Flags().StringVar(&listFolder, "folder", "INBOX", "Folder to read from")
	mailListCmd.Flags().Uint32Var(&listLimit, "limit", 10, "Maximum number of emails")
	mailListCmd.Flags().BoolVar(&listUnreadOnly, "unread", false, "Only unread emails")
	mailListCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	// Read flags
	readCmd.Flags().StringVar(&readFolder, "folder", "INBOX", "Folder of the email")

	// Send flags
	sendCmd.Flags().StringArrayVar(&sendTo, "to", nil, "Recipients (can be specified multiple times)")
	sendCmd.Flags().StringArrayVar(&sendCc, "cc", nil, "CC recipients")
	sendCmd.Flags().StringArrayVar(&sendBcc, "bcc", nil, "BCC recipients")
	sendCmd.Flags().StringVar(&sendSubject, "subject", "", "Subject")
	sendCmd.Flags().StringVar(&sendBody, "body", "", "Message body")
	sendCmd.Flags().StringVar(&sendBodyFile, "body-file", "", "Read message body from file")
	sendCmd.Flags().BoolVar(&sendHTML, "html", false, "Send body as HTML")
	sendCmd.Flags().StringArrayVar(&sendAttach, "attach", nil, "Attachments (file paths)")

	sendCmd.MarkFlagRequired("to")
	sendCmd.MarkFlagRequired("subject")

	mailCmd.AddCommand(mailListCmd)
	mailCmd.AddCommand(readCmd)
	mailCmd.AddCommand(sendCmd)
}

func runMailList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get active account
	email := getActiveAccount()
	if email == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Get OAuth token
	oauthClient, err := auth.NewOAuthClient(cfg.ClientID, cfg.CacheDir)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	accessToken, err := oauthClient.GetAccessToken(ctx, email)
	if err != nil {
		return fmt.Errorf("not logged in. Please run 'auth login': %w", err)
	}

	// IMAP Client
	imapClient := mail.NewIMAPClient(oauthClient, email, cfg.IMAPServer, cfg.IMAPPort)

	debugLog("Connecting to IMAP server %s:%d", cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}
	defer imapClient.Close()

	debugLog("IMAP connection established, reading folder %s", listFolder)

	// Fetch emails
	emails, err := imapClient.ListEmails(listFolder, listLimit, listUnreadOnly)
	if err != nil {
		return err
	}

	// Output
	if listJSON {
		return outputJSON(emails)
	}

	if len(emails) == 0 {
		printInfo("No emails found.")
		return nil
	}

	fmt.Printf("\n%-8s %-20s %-30s %s\n", "UID", "Date", "From", "Subject")
	fmt.Println(strings.Repeat("─", 100))

	for _, email := range emails {
		unreadMarker := " "
		if email.Unread {
			unreadMarker = "●"
		}

		from := truncate(email.From, 28)
		subject := truncate(email.Subject, 35)
		date := email.Date.Local().Format("2006-01-02 15:04")

		fmt.Printf("%s %-7d %-20s %-30s %s\n", unreadMarker, email.UID, date, from, subject)
	}

	fmt.Printf("\n%d emails shown\n", len(emails))

	return nil
}

func runRead(cmd *cobra.Command, args []string) error {
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

	// Fetch email
	email, err := imapClient.GetEmail(readFolder, uid)
	if err != nil {
		return err
	}

	// Display
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("From:    %s\n", email.From)
	fmt.Printf("To:      %s\n", strings.Join(email.To, ", "))
	fmt.Printf("Subject: %s\n", email.Subject)
	fmt.Printf("Date:    %s\n", email.Date.Local().Format(time.RFC1123))
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println(email.Body)

	return nil
}

func runSend(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validation
	if len(sendTo) == 0 {
		return fmt.Errorf("at least one recipient (--to) required")
	}

	// Body from file or direct
	body := sendBody
	if sendBodyFile != "" {
		content, err := os.ReadFile(sendBodyFile)
		if err != nil {
			return fmt.Errorf("could not read body file: %w", err)
		}
		body = string(content)
	}

	if body == "" {
		return fmt.Errorf("message body required (--body or --body-file)")
	}

	// Get active account
	email := getActiveAccount()
	if email == "" {
		return fmt.Errorf("no account configured. Please run 'auth login'")
	}

	// Check attachments
	for _, attach := range sendAttach {
		if _, err := os.Stat(attach); err != nil {
			return fmt.Errorf("attachment not found: %s", attach)
		}
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

	// SMTP Client
	smtpClient := mail.NewSMTPClient(email, cfg.SMTPServer, cfg.SMTPPort)

	debugLog("Sending email via %s:%d", cfg.SMTPServer, cfg.SMTPPort)

	// Send
	opts := mail.SendOptions{
		To:          sendTo,
		Cc:          sendCc,
		Bcc:         sendBcc,
		Subject:     sendSubject,
		Body:        body,
		HTML:        sendHTML,
		Attachments: sendAttach,
	}

	if err := smtpClient.Send(accessToken, opts); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	printSuccess("Email sent to %s", strings.Join(sendTo, ", "))
	if len(sendCc) > 0 {
		printInfo("CC: %s", strings.Join(sendCc, ", "))
	}
	if len(sendAttach) > 0 {
		printInfo("With %d attachment(s)", len(sendAttach))
	}

	return nil
}

// Helper functions

func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
