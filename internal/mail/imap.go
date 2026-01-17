package mail

import (
	"fmt"
	"io"
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
