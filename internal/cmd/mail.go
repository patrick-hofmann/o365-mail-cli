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

// Mark-read Command
var markReadFolder string

var markReadCmd = &cobra.Command{
	Use:   "mark-read [uid]",
	Short: "Mark email as read",
	Long: `Marks an email as read by adding the \Seen flag.

Examples:
  o365-mail-cli mail mark-read 12345
  o365-mail-cli mail mark-read 12345 --folder "Archive"`,
	Args: cobra.ExactArgs(1),
	RunE: runMarkRead,
}

// Mark-unread Command
var markUnreadFolder string

var markUnreadCmd = &cobra.Command{
	Use:   "mark-unread [uid]",
	Short: "Mark email as unread",
	Long: `Marks an email as unread by removing the \Seen flag.

Examples:
  o365-mail-cli mail mark-unread 12345
  o365-mail-cli mail mark-unread 12345 --folder "Archive"`,
	Args: cobra.ExactArgs(1),
	RunE: runMarkUnread,
}

// Move Command
var (
	moveFromFolder string
	moveToFolder   string
)

var moveCmd = &cobra.Command{
	Use:   "move [uid]",
	Short: "Move email to folder",
	Long: `Moves an email to another folder.

Examples:
  o365-mail-cli mail move 12345 --to "Archive"
  o365-mail-cli mail move 12345 --folder "Sent Items" --to "Archive/2024"`,
	Args: cobra.ExactArgs(1),
	RunE: runMove,
}

// Trash Command
var trashFolder string

var trashCmd = &cobra.Command{
	Use:   "trash [uid]",
	Short: "Move email to Trash",
	Long: `Moves an email to the Trash folder (Deleted Items).
This is a safe delete - the email can be recovered from Trash.

Examples:
  o365-mail-cli mail trash 12345
  o365-mail-cli mail trash 12345 --folder "Spam"`,
	Args: cobra.ExactArgs(1),
	RunE: runTrash,
}

// Search Command
var (
	searchFolder  string
	searchFrom    string
	searchSubject string
	searchSince   string
	searchLimit   uint32
	searchJSON    bool
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search emails",
	Long: `Searches emails by various criteria.

Examples:
  o365-mail-cli mail search --from "sender@example.com"
  o365-mail-cli mail search --subject "important"
  o365-mail-cli mail search --since 24h
  o365-mail-cli mail search --from "boss@company.com" --since 7d --json`,
	RunE: runSearch,
}

// Attachments Command
var (
	attachFolder string
	attachSaveTo string
)

var attachmentsCmd = &cobra.Command{
	Use:   "attachments [uid]",
	Short: "Download email attachments",
	Long: `Downloads attachments from an email.

Examples:
  o365-mail-cli mail attachments 12345 --save-to ./downloads
  o365-mail-cli mail attachments 12345 --folder "Sent Items" --save-to /tmp/attachments`,
	Args: cobra.ExactArgs(1),
	RunE: runAttachments,
}

// Reply Command
var (
	replyFolder   string
	replyBody     string
	replyBodyFile string
	replyAll      bool
)

var replyCmd = &cobra.Command{
	Use:   "reply [uid]",
	Short: "Reply to an email",
	Long: `Replies to an email with proper threading headers.

Examples:
  o365-mail-cli mail reply 12345 --body "Thank you for your email!"
  o365-mail-cli mail reply 12345 --body-file response.txt
  o365-mail-cli mail reply 12345 --body "Thanks!" --reply-all`,
	Args: cobra.ExactArgs(1),
	RunE: runReply,
}

// Forward Command
var (
	forwardFolder   string
	forwardTo       []string
	forwardBody     string
	forwardBodyFile string
)

var forwardCmd = &cobra.Command{
	Use:   "forward [uid]",
	Short: "Forward an email",
	Long: `Forwards an email to new recipients.

Examples:
  o365-mail-cli mail forward 12345 --to colleague@example.com
  o365-mail-cli mail forward 12345 --to colleague@example.com --body "FYI - please review"`,
	Args: cobra.ExactArgs(1),
	RunE: runForward,
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

	// Mark-read flags
	markReadCmd.Flags().StringVar(&markReadFolder, "folder", "INBOX", "Folder of the email")

	// Mark-unread flags
	markUnreadCmd.Flags().StringVar(&markUnreadFolder, "folder", "INBOX", "Folder of the email")

	// Move flags
	moveCmd.Flags().StringVar(&moveFromFolder, "folder", "INBOX", "Source folder")
	moveCmd.Flags().StringVar(&moveToFolder, "to", "", "Destination folder")
	moveCmd.MarkFlagRequired("to")

	// Trash flags
	trashCmd.Flags().StringVar(&trashFolder, "folder", "INBOX", "Folder of the email")

	// Search flags
	searchCmd.Flags().StringVar(&searchFolder, "folder", "INBOX", "Folder to search")
	searchCmd.Flags().StringVar(&searchFrom, "from", "", "Filter by sender")
	searchCmd.Flags().StringVar(&searchSubject, "subject", "", "Filter by subject")
	searchCmd.Flags().StringVar(&searchSince, "since", "", "Filter emails since (e.g., 24h, 7d, 30d)")
	searchCmd.Flags().Uint32Var(&searchLimit, "limit", 50, "Maximum results")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output as JSON")

	// Attachments flags
	attachmentsCmd.Flags().StringVar(&attachFolder, "folder", "INBOX", "Folder of the email")
	attachmentsCmd.Flags().StringVar(&attachSaveTo, "save-to", "", "Directory to save attachments")
	attachmentsCmd.MarkFlagRequired("save-to")

	// Reply flags
	replyCmd.Flags().StringVar(&replyFolder, "folder", "INBOX", "Folder of the email")
	replyCmd.Flags().StringVar(&replyBody, "body", "", "Reply message body")
	replyCmd.Flags().StringVar(&replyBodyFile, "body-file", "", "Read reply body from file")
	replyCmd.Flags().BoolVar(&replyAll, "reply-all", false, "Reply to all recipients")

	// Forward flags
	forwardCmd.Flags().StringVar(&forwardFolder, "folder", "INBOX", "Folder of the email")
	forwardCmd.Flags().StringArrayVar(&forwardTo, "to", nil, "Recipients (can be specified multiple times)")
	forwardCmd.Flags().StringVar(&forwardBody, "body", "", "Additional message body")
	forwardCmd.Flags().StringVar(&forwardBodyFile, "body-file", "", "Read additional body from file")
	forwardCmd.MarkFlagRequired("to")

	mailCmd.AddCommand(mailListCmd)
	mailCmd.AddCommand(readCmd)
	mailCmd.AddCommand(sendCmd)
	mailCmd.AddCommand(markReadCmd)
	mailCmd.AddCommand(markUnreadCmd)
	mailCmd.AddCommand(moveCmd)
	mailCmd.AddCommand(trashCmd)
	mailCmd.AddCommand(searchCmd)
	mailCmd.AddCommand(attachmentsCmd)
	mailCmd.AddCommand(replyCmd)
	mailCmd.AddCommand(forwardCmd)
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

func runMarkRead(cmd *cobra.Command, args []string) error {
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

	// Mark as read
	if err := imapClient.MarkAsRead(markReadFolder, uid); err != nil {
		return err
	}

	printSuccess("Email %d marked as read", uid)
	return nil
}

func runMarkUnread(cmd *cobra.Command, args []string) error {
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

	// Mark as unread
	if err := imapClient.MarkAsUnread(markUnreadFolder, uid); err != nil {
		return err
	}

	printSuccess("Email %d marked as unread", uid)
	return nil
}

func runMove(cmd *cobra.Command, args []string) error {
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

	// Move email
	if err := imapClient.MoveEmail(moveFromFolder, moveToFolder, uid); err != nil {
		return err
	}

	printSuccess("Email %d moved from '%s' to '%s'", uid, moveFromFolder, moveToFolder)
	return nil
}

func runTrash(cmd *cobra.Command, args []string) error {
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

	// Trash email
	if err := imapClient.TrashEmail(trashFolder, uid); err != nil {
		return err
	}

	printSuccess("Email %d moved to Trash", uid)
	return nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Check that at least one search criterion is provided
	if searchFrom == "" && searchSubject == "" && searchSince == "" {
		return fmt.Errorf("at least one search criterion required (--from, --subject, or --since)")
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

	// Build search criteria
	criteria := mail.SearchCriteria{
		From:    searchFrom,
		Subject: searchSubject,
	}

	// Parse since duration
	if searchSince != "" {
		duration, err := parseDuration(searchSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		criteria.Since = time.Now().Add(-duration)
	}

	// Search
	emails, err := imapClient.SearchEmails(searchFolder, criteria, searchLimit)
	if err != nil {
		return err
	}

	// Output
	if searchJSON {
		return outputJSON(emails)
	}

	if len(emails) == 0 {
		printInfo("No emails found matching criteria.")
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

	fmt.Printf("\n%d emails found\n", len(emails))

	return nil
}

func runAttachments(cmd *cobra.Command, args []string) error {
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

	// Get attachments
	attachments, err := imapClient.GetAttachments(attachFolder, uid, attachSaveTo)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		printInfo("No attachments found in email %d", uid)
		return nil
	}

	printSuccess("Downloaded %d attachment(s) to %s:", len(attachments), attachSaveTo)
	for _, att := range attachments {
		fmt.Printf("  - %s (%s, %d bytes)\n", att.Filename, att.ContentType, att.Size)
	}

	return nil
}

func runReply(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse UID
	var uid uint32
	if _, err := fmt.Sscanf(args[0], "%d", &uid); err != nil {
		return fmt.Errorf("invalid UID: %s", args[0])
	}

	// Get body from file or direct
	body := replyBody
	if replyBodyFile != "" {
		content, err := os.ReadFile(replyBodyFile)
		if err != nil {
			return fmt.Errorf("could not read body file: %w", err)
		}
		body = string(content)
	}

	if body == "" {
		return fmt.Errorf("reply body required (--body or --body-file)")
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

	// IMAP Client - fetch original email
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}

	originalEmail, err := imapClient.GetEmail(replyFolder, uid)
	if err != nil {
		imapClient.Close()
		return fmt.Errorf("failed to fetch original email: %w", err)
	}
	imapClient.Close()

	// SMTP Client
	smtpClient := mail.NewSMTPClient(account, cfg.SMTPServer, cfg.SMTPPort)

	debugLog("Sending reply via %s:%d", cfg.SMTPServer, cfg.SMTPPort)

	// Build reply options
	opts := mail.ReplyOptions{
		OriginalMessageID: originalEmail.MessageID,
		OriginalFrom:      originalEmail.From,
		OriginalTo:        originalEmail.To,
		OriginalSubject:   originalEmail.Subject,
		OriginalDate:      originalEmail.Date,
		OriginalBody:      originalEmail.Body,
		Body:              body,
		ReplyAll:          replyAll,
	}

	if err := smtpClient.Reply(accessToken, opts); err != nil {
		return fmt.Errorf("reply failed: %w", err)
	}

	if replyAll {
		printSuccess("Reply-all sent for email %d", uid)
	} else {
		printSuccess("Reply sent to %s", originalEmail.From)
	}

	return nil
}

func runForward(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse UID
	var uid uint32
	if _, err := fmt.Sscanf(args[0], "%d", &uid); err != nil {
		return fmt.Errorf("invalid UID: %s", args[0])
	}

	// Validate recipients
	if len(forwardTo) == 0 {
		return fmt.Errorf("at least one recipient (--to) required")
	}

	// Get body from file or direct
	body := forwardBody
	if forwardBodyFile != "" {
		content, err := os.ReadFile(forwardBodyFile)
		if err != nil {
			return fmt.Errorf("could not read body file: %w", err)
		}
		body = string(content)
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

	// IMAP Client - fetch original email
	imapClient := mail.NewIMAPClient(oauthClient, account, cfg.IMAPServer, cfg.IMAPPort)

	if err := imapClient.Connect(accessToken); err != nil {
		return err
	}

	originalEmail, err := imapClient.GetEmail(forwardFolder, uid)
	if err != nil {
		imapClient.Close()
		return fmt.Errorf("failed to fetch original email: %w", err)
	}
	imapClient.Close()

	// SMTP Client
	smtpClient := mail.NewSMTPClient(account, cfg.SMTPServer, cfg.SMTPPort)

	debugLog("Forwarding email via %s:%d", cfg.SMTPServer, cfg.SMTPPort)

	// Build forward options
	opts := mail.ForwardOptions{
		OriginalFrom:    originalEmail.From,
		OriginalTo:      originalEmail.To,
		OriginalSubject: originalEmail.Subject,
		OriginalDate:    originalEmail.Date,
		OriginalBody:    originalEmail.Body,
		To:              forwardTo,
		Body:            body,
	}

	if err := smtpClient.Forward(accessToken, opts); err != nil {
		return fmt.Errorf("forward failed: %w", err)
	}

	printSuccess("Email %d forwarded to %s", uid, strings.Join(forwardTo, ", "))

	return nil
}

// Helper functions

// parseDuration parses duration strings like "24h", "7d", "30d"
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Handle days specially
	if strings.HasSuffix(s, "d") {
		days := s[:len(s)-1]
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid days: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// Use standard time.ParseDuration for hours, minutes, seconds
	return time.ParseDuration(s)
}

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
