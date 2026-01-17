package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-sasl"
	"github.com/yourname/o365-mail-cli/internal/auth"
)

const (
	DefaultIMAPServer = "outlook.office365.com"
	DefaultIMAPPort   = 993
)

// IMAPClient wraps the IMAP connection with OAuth2 support
type IMAPClient struct {
	client      *client.Client
	email       string
	server      string
	port        int
	oauthClient *auth.OAuthClient
}

// xoauth2Client implements the XOAUTH2 SASL mechanism
type xoauth2Client struct {
	email string
	token string
}

func (x *xoauth2Client) Start() (mech string, ir []byte, err error) {
	// XOAUTH2 Format: user=<email>\x01auth=Bearer <token>\x01\x01
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", x.email, x.token)
	return "XOAUTH2", []byte(authStr), nil
}

func (x *xoauth2Client) Next(challenge []byte) (response []byte, err error) {
	// XOAUTH2 has no challenge-response
	return nil, nil
}

// NewIMAPClient creates a new IMAP client
func NewIMAPClient(oauthClient *auth.OAuthClient, email, server string, port int) *IMAPClient {
	if server == "" {
		server = DefaultIMAPServer
	}
	if port == 0 {
		port = DefaultIMAPPort
	}

	return &IMAPClient{
		email:       email,
		server:      server,
		port:        port,
		oauthClient: oauthClient,
	}
}

// Connect establishes the IMAP connection and authenticates with OAuth2
func (c *IMAPClient) Connect(accessToken string) error {
	// Establish TLS connection
	addr := fmt.Sprintf("%s:%d", c.server, c.port)
	imapClient, err := client.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	c.client = imapClient

	// XOAUTH2 authentication
	saslClient := &xoauth2Client{
		email: c.email,
		token: accessToken,
	}

	if err := c.client.Authenticate(saslClient); err != nil {
		c.client.Logout()
		return fmt.Errorf("IMAP authentication failed: %w", err)
	}

	return nil
}

// Close closes the IMAP connection
func (c *IMAPClient) Close() error {
	if c.client != nil {
		return c.client.Logout()
	}
	return nil
}

// Email represents an email message
type Email struct {
	UID         uint32    `json:"uid"`
	MessageID   string    `json:"message_id"`
	Date        time.Time `json:"date"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	Flags       []string  `json:"flags"`
	Size        uint32    `json:"size"`
	Preview     string    `json:"preview,omitempty"`
	Body        string    `json:"body,omitempty"`
	Unread      bool      `json:"unread"`
}

// ListEmails lists emails from a folder
func (c *IMAPClient) ListEmails(folder string, limit uint32, unreadOnly bool) ([]Email, error) {
	if folder == "" {
		folder = "INBOX"
	}

	// Select folder
	mbox, err := c.client.Select(folder, true) // readonly
	if err != nil {
		return nil, fmt.Errorf("failed to select folder '%s': %w", folder, err)
	}

	if mbox.Messages == 0 {
		return []Email{}, nil
	}

	// Calculate the range of the last N messages
	from := uint32(1)
	if mbox.Messages > limit {
		from = mbox.Messages - limit + 1
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(from, mbox.Messages)

	// If only unread emails
	if unreadOnly {
		criteria := imap.NewSearchCriteria()
		criteria.WithoutFlags = []string{imap.SeenFlag}
		uids, err := c.client.Search(criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to search: %w", err)
		}
		if len(uids) == 0 {
			return []Email{}, nil
		}
		seqSet = new(imap.SeqSet)
		// Only the last N unread messages
		start := 0
		if len(uids) > int(limit) {
			start = len(uids) - int(limit)
		}
		seqSet.AddNum(uids[start:]...)
	}

	// Fields we want to fetch
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchRFC822Size,
	}

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)

	go func() {
		done <- c.client.Fetch(seqSet, items, messages)
	}()

	var emails []Email
	for msg := range messages {
		email := Email{
			UID:   msg.Uid,
			Size:  msg.Size,
			Flags: msg.Flags,
		}

		if msg.Envelope != nil {
			email.MessageID = msg.Envelope.MessageId
			email.Subject = msg.Envelope.Subject
			email.Date = msg.Envelope.Date

			if len(msg.Envelope.From) > 0 {
				email.From = formatAddress(msg.Envelope.From[0])
			}

			for _, addr := range msg.Envelope.To {
				email.To = append(email.To, formatAddress(addr))
			}
		}

		// Check if unread
		email.Unread = true
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				email.Unread = false
				break
			}
		}

		emails = append(emails, email)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	// Newest first
	reverseEmails(emails)

	return emails, nil
}

// GetEmail fetches a single email with body
func (c *IMAPClient) GetEmail(folder string, uid uint32) (*Email, error) {
	if folder == "" {
		folder = "INBOX"
	}

	_, err := c.client.Select(folder, true)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Fetch everything including body
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		section.FetchItem(),
	}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)

	go func() {
		done <- c.client.UidFetch(seqSet, items, messages)
	}()

	var email *Email
	for msg := range messages {
		email = &Email{
			UID:   msg.Uid,
			Flags: msg.Flags,
		}

		if msg.Envelope != nil {
			email.MessageID = msg.Envelope.MessageId
			email.Subject = msg.Envelope.Subject
			email.Date = msg.Envelope.Date

			if len(msg.Envelope.From) > 0 {
				email.From = formatAddress(msg.Envelope.From[0])
			}

			for _, addr := range msg.Envelope.To {
				email.To = append(email.To, formatAddress(addr))
			}
		}

		// Read body
		for _, literal := range msg.Body {
			if literal != nil {
				body, err := io.ReadAll(literal)
				if err == nil {
					email.Body = string(body)
				}
			}
		}

		email.Unread = true
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				email.Unread = false
				break
			}
		}
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch message: %w", err)
	}

	if email == nil {
		return nil, fmt.Errorf("message not found")
	}

	return email, nil
}

// ListFolders lists all available folders
func (c *IMAPClient) ListFolders() ([]Folder, error) {
	mailboxes := make(chan *imap.MailboxInfo, 100)
	done := make(chan error, 1)

	go func() {
		done <- c.client.List("", "*", mailboxes)
	}()

	var folders []Folder
	for mbox := range mailboxes {
		folders = append(folders, Folder{
			Name:       mbox.Name,
			Delimiter:  mbox.Delimiter,
			Attributes: mbox.Attributes,
		})
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, nil
}

// Folder represents an IMAP folder
type Folder struct {
	Name       string   `json:"name"`
	Delimiter  string   `json:"delimiter"`
	Attributes []string `json:"attributes"`
}

// CreateFolder creates a new folder
func (c *IMAPClient) CreateFolder(name string) error {
	if err := c.client.Create(name); err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}
	return nil
}

// DeleteFolder deletes a folder
func (c *IMAPClient) DeleteFolder(name string) error {
	if err := c.client.Delete(name); err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	return nil
}

// Helper Functions

func formatAddress(addr *imap.Address) string {
	if addr == nil {
		return ""
	}
	if addr.PersonalName != "" {
		return fmt.Sprintf("%s <%s@%s>", addr.PersonalName, addr.MailboxName, addr.HostName)
	}
	return fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
}

func reverseEmails(emails []Email) {
	for i, j := 0, len(emails)-1; i < j; i, j = i+1, j-1 {
		emails[i], emails[j] = emails[j], emails[i]
	}
}

// NewXOAuth2Client creates a SASL client for XOAUTH2
func NewXOAuth2Client(email, token string) sasl.Client {
	return &xoauth2Client{
		email: email,
		token: token,
	}
}

// ParseEmail extracts the email address from an address string
func ParseEmail(address string) string {
	// Format: "Name <email@domain.com>" or "email@domain.com"
	if idx := strings.Index(address, "<"); idx != -1 {
		end := strings.Index(address, ">")
		if end > idx {
			return address[idx+1 : end]
		}
	}
	return strings.TrimSpace(address)
}

// MarkAsRead marks an email as read by adding the \Seen flag
func (c *IMAPClient) MarkAsRead(folder string, uid uint32) error {
	if folder == "" {
		folder = "INBOX"
	}

	// Select folder in read-write mode
	_, err := c.client.Select(folder, false) // readonly=false
	if err != nil {
		return fmt.Errorf("failed to select folder '%s': %w", folder, err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Add \Seen flag
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}

	if err := c.client.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark email as read: %w", err)
	}

	return nil
}

// MarkAsUnread marks an email as unread by removing the \Seen flag
func (c *IMAPClient) MarkAsUnread(folder string, uid uint32) error {
	if folder == "" {
		folder = "INBOX"
	}

	// Select folder in read-write mode
	_, err := c.client.Select(folder, false) // readonly=false
	if err != nil {
		return fmt.Errorf("failed to select folder '%s': %w", folder, err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Remove \Seen flag
	item := imap.FormatFlagsOp(imap.RemoveFlags, true)
	flags := []interface{}{imap.SeenFlag}

	if err := c.client.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark email as unread: %w", err)
	}

	return nil
}

// MoveEmail moves an email to another folder (copy + delete from source)
func (c *IMAPClient) MoveEmail(srcFolder, dstFolder string, uid uint32) error {
	if srcFolder == "" {
		srcFolder = "INBOX"
	}

	// Select source folder in read-write mode
	_, err := c.client.Select(srcFolder, false) // readonly=false
	if err != nil {
		return fmt.Errorf("failed to select source folder '%s': %w", srcFolder, err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Copy to destination folder
	if err := c.client.UidCopy(seqSet, dstFolder); err != nil {
		return fmt.Errorf("failed to copy email to '%s': %w", dstFolder, err)
	}

	// Mark as deleted in source folder
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	if err := c.client.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark email for deletion: %w", err)
	}

	// Expunge to permanently remove from source
	if err := c.client.Expunge(nil); err != nil {
		return fmt.Errorf("failed to expunge: %w", err)
	}

	return nil
}

// TrashEmail moves an email to the Trash folder (safe delete)
func (c *IMAPClient) TrashEmail(folder string, uid uint32) error {
	// Office 365 uses "Deleted Items" as the trash folder
	return c.MoveEmail(folder, "Deleted Items", uid)
}

// SearchCriteria contains search parameters for emails
type SearchCriteria struct {
	From    string
	Subject string
	Since   time.Time
	Before  time.Time
}

// SearchEmails searches emails by criteria
func (c *IMAPClient) SearchEmails(folder string, criteria SearchCriteria, limit uint32) ([]Email, error) {
	if folder == "" {
		folder = "INBOX"
	}

	// Select folder
	_, err := c.client.Select(folder, true) // readonly
	if err != nil {
		return nil, fmt.Errorf("failed to select folder '%s': %w", folder, err)
	}

	// Build search criteria
	searchCriteria := imap.NewSearchCriteria()

	if criteria.From != "" {
		searchCriteria.Header.Add("From", criteria.From)
	}

	if criteria.Subject != "" {
		searchCriteria.Header.Add("Subject", criteria.Subject)
	}

	if !criteria.Since.IsZero() {
		searchCriteria.Since = criteria.Since
	}

	if !criteria.Before.IsZero() {
		searchCriteria.Before = criteria.Before
	}

	// Execute search
	uids, err := c.client.Search(searchCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if len(uids) == 0 {
		return []Email{}, nil
	}

	// Limit results
	start := 0
	if len(uids) > int(limit) {
		start = len(uids) - int(limit)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids[start:]...)

	// Fetch emails
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchRFC822Size,
	}

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)

	go func() {
		done <- c.client.Fetch(seqSet, items, messages)
	}()

	var emails []Email
	for msg := range messages {
		email := Email{
			UID:   msg.Uid,
			Size:  msg.Size,
			Flags: msg.Flags,
		}

		if msg.Envelope != nil {
			email.MessageID = msg.Envelope.MessageId
			email.Subject = msg.Envelope.Subject
			email.Date = msg.Envelope.Date

			if len(msg.Envelope.From) > 0 {
				email.From = formatAddress(msg.Envelope.From[0])
			}

			for _, addr := range msg.Envelope.To {
				email.To = append(email.To, formatAddress(addr))
			}
		}

		email.Unread = true
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				email.Unread = false
				break
			}
		}

		emails = append(emails, email)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	// Newest first
	reverseEmails(emails)

	return emails, nil
}

// Attachment represents an email attachment
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	SavedPath   string `json:"saved_path,omitempty"`
}

// GetAttachments extracts and saves attachments from an email
func (c *IMAPClient) GetAttachments(folder string, uid uint32, saveDir string) ([]Attachment, error) {
	if folder == "" {
		folder = "INBOX"
	}

	_, err := c.client.Select(folder, true)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Fetch full body
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem()}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)

	go func() {
		done <- c.client.UidFetch(seqSet, items, messages)
	}()

	var body []byte
	for msg := range messages {
		for _, literal := range msg.Body {
			if literal != nil {
				body, _ = io.ReadAll(literal)
			}
		}
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch message: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("message not found")
	}

	// Parse MIME structure
	return extractAttachments(body, saveDir)
}

// extractAttachments parses the email body and extracts attachments
func extractAttachments(body []byte, saveDir string) ([]Attachment, error) {
	// Find Content-Type header
	reader := bytes.NewReader(body)
	buf := make([]byte, len(body))
	reader.Read(buf)

	// Simple header parsing to find Content-Type
	headerEnd := bytes.Index(buf, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		headerEnd = bytes.Index(buf, []byte("\n\n"))
	}
	if headerEnd == -1 {
		return nil, fmt.Errorf("invalid email format")
	}

	headers := string(buf[:headerEnd])
	bodyContent := buf[headerEnd+4:]

	// Extract Content-Type
	contentType := ""
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "content-type:") {
			contentType = strings.TrimSpace(line[13:])
			// Handle multi-line headers
			break
		}
	}

	if contentType == "" {
		return []Attachment{}, nil
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return []Attachment{}, nil
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return []Attachment{}, nil
	}

	boundary := params["boundary"]
	if boundary == "" {
		return []Attachment{}, nil
	}

	// Parse multipart
	mr := multipart.NewReader(bytes.NewReader(bodyContent), boundary)

	var attachments []Attachment
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		disposition := part.Header.Get("Content-Disposition")
		if !strings.Contains(strings.ToLower(disposition), "attachment") {
			part.Close()
			continue
		}

		// Extract filename
		_, dispParams, _ := mime.ParseMediaType(disposition)
		filename := dispParams["filename"]
		if filename == "" {
			filename = "attachment"
		}

		// Read content
		content, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			continue
		}

		// Decode if needed
		encoding := part.Header.Get("Content-Transfer-Encoding")
		decoded := decodeContent(content, encoding)

		// Get content type
		partContentType := part.Header.Get("Content-Type")
		if partContentType == "" {
			partContentType = "application/octet-stream"
		}
		mt, _, _ := mime.ParseMediaType(partContentType)
		if mt != "" {
			partContentType = mt
		}

		attachment := Attachment{
			Filename:    filename,
			ContentType: partContentType,
			Size:        len(decoded),
		}

		// Save if directory provided
		if saveDir != "" {
			if err := os.MkdirAll(saveDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}

			savePath := filepath.Join(saveDir, filename)
			if err := os.WriteFile(savePath, decoded, 0644); err != nil {
				return nil, fmt.Errorf("failed to save attachment: %w", err)
			}
			attachment.SavedPath = savePath
		}

		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

// decodeContent decodes content based on transfer encoding
func decodeContent(content []byte, encoding string) []byte {
	switch strings.ToLower(encoding) {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(string(content))
		if err != nil {
			return content
		}
		return decoded
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(content))
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return content
		}
		return decoded
	default:
		return content
	}
}

// DraftEmail represents an email draft
type DraftEmail struct {
	From    string
	To      []string
	Cc      []string
	Subject string
	Body    string
	HTML    bool
}

// SaveDraft saves an email draft to the Drafts folder
func (c *IMAPClient) SaveDraft(draft DraftEmail) error {
	// Build RFC 5322 message
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("From: %s\r\n", draft.From))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(draft.To, ", ")))

	if len(draft.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(draft.Cc, ", ")))
	}

	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", draft.Subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	contentType := "text/plain; charset=utf-8"
	if draft.HTML {
		contentType = "text/html; charset=utf-8"
	}
	buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	buf.WriteString("\r\n")
	buf.WriteString(draft.Body)

	// Append to Drafts folder with \Draft flag
	flags := []string{imap.DraftFlag}
	literal := bytes.NewReader(buf.Bytes())

	if err := c.client.Append("Drafts", flags, time.Now(), literal); err != nil {
		return fmt.Errorf("failed to save draft: %w", err)
	}

	return nil
}

// ListDrafts lists emails in the Drafts folder
func (c *IMAPClient) ListDrafts(limit uint32) ([]Email, error) {
	return c.ListEmails("Drafts", limit, false)
}

// DeleteDraft removes a draft from the Drafts folder
func (c *IMAPClient) DeleteDraft(uid uint32) error {
	// Select Drafts folder in read-write mode
	_, err := c.client.Select("Drafts", false)
	if err != nil {
		return fmt.Errorf("failed to select Drafts folder: %w", err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Mark as deleted
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	if err := c.client.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark draft for deletion: %w", err)
	}

	// Expunge
	if err := c.client.Expunge(nil); err != nil {
		return fmt.Errorf("failed to expunge: %w", err)
	}

	return nil
}
