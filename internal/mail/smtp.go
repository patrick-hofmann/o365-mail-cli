package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultSMTPServer = "smtp.office365.com"
	DefaultSMTPPort   = 587
)

// SMTPClient for sending emails with OAuth2
type SMTPClient struct {
	email  string
	server string
	port   int
}

// NewSMTPClient creates a new SMTP client
func NewSMTPClient(email, server string, port int) *SMTPClient {
	if server == "" {
		server = DefaultSMTPServer
	}
	if port == 0 {
		port = DefaultSMTPPort
	}

	return &SMTPClient{
		email:  email,
		server: server,
		port:   port,
	}
}

// SendOptions contains options for sending emails
type SendOptions struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	HTML        bool
	Attachments []string
}

// Send sends an email via SMTP with XOAUTH2
func (c *SMTPClient) Send(accessToken string, opts SendOptions) error {
	// Establish connection
	addr := fmt.Sprintf("%s:%d", c.server, c.port)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, c.server)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Send EHLO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}

	// STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: c.server}
		if err := client.StartTLS(config); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	// XOAUTH2 authentication
	auth := &xoauth2SMTPAuth{
		email: c.email,
		token: accessToken,
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender
	if err := client.Mail(c.email); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}

	// Set recipients
	allRecipients := append(append(opts.To, opts.Cc...), opts.Bcc...)
	for _, rcpt := range allRecipients {
		email := ParseEmail(rcpt)
		if err := client.Rcpt(email); err != nil {
			return fmt.Errorf("RCPT TO failed for %s: %w", email, err)
		}
	}

	// Send email content
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}

	msg, err := c.buildMessage(opts)
	if err != nil {
		wc.Close()
		return fmt.Errorf("failed to build message: %w", err)
	}

	if _, err := wc.Write(msg); err != nil {
		wc.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// Close connection cleanly
	return client.Quit()
}

// buildMessage creates the email message in RFC 5322 format
func (c *SMTPClient) buildMessage(opts SendOptions) ([]byte, error) {
	var buf bytes.Buffer

	// Header
	buf.WriteString(fmt.Sprintf("From: %s\r\n", c.email))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(opts.To, ", ")))
	
	if len(opts.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(opts.Cc, ", ")))
	}
	
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeSubject(opts.Subject)))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	// With or without attachments
	if len(opts.Attachments) > 0 {
		return c.buildMultipartMessage(&buf, opts)
	}

	// Simple message without attachments
	contentType := "text/plain; charset=utf-8"
	if opts.HTML {
		contentType = "text/html; charset=utf-8"
	}
	buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(opts.Body)

	return buf.Bytes(), nil
}

// buildMultipartMessage creates an email with attachments
func (c *SMTPClient) buildMultipartMessage(header *bytes.Buffer, opts SendOptions) ([]byte, error) {
	var buf bytes.Buffer

	// Create multipart writer
	writer := multipart.NewWriter(&buf)

	// Add Content-Type header
	header.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n", writer.Boundary()))
	header.WriteString("\r\n")

	// Body part
	contentType := "text/plain; charset=utf-8"
	if opts.HTML {
		contentType = "text/html; charset=utf-8"
	}

	bodyHeader := make(textproto.MIMEHeader)
	bodyHeader.Set("Content-Type", contentType)
	bodyHeader.Set("Content-Transfer-Encoding", "quoted-printable")

	bodyPart, err := writer.CreatePart(bodyHeader)
	if err != nil {
		return nil, err
	}
	bodyPart.Write([]byte(opts.Body))

	// Attachments
	for _, attachment := range opts.Attachments {
		if err := c.addAttachment(writer, attachment); err != nil {
			return nil, fmt.Errorf("failed to add attachment %s: %w", attachment, err)
		}
	}

	writer.Close()

	// Combine header and body
	var result bytes.Buffer
	result.Write(header.Bytes())
	result.Write(buf.Bytes())

	return result.Bytes(), nil
}

// addAttachment adds an attachment to the email
func (c *SMTPClient) addAttachment(writer *multipart.Writer, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	filename := filepath.Base(filePath)

	// Determine content type
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Header for attachment
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "base64")
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}

	// Encode file in Base64
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(content)

	// Split into 76-character blocks (RFC 2045)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		part.Write([]byte(encoded[i:end] + "\r\n"))
	}

	return nil
}

// xoauth2SMTPAuth implements smtp.Auth for XOAUTH2
type xoauth2SMTPAuth struct {
	email string
	token string
}

func (a *xoauth2SMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// XOAUTH2 Format: user=<email>\x01auth=Bearer <token>\x01\x01
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.email, a.token)
	return "XOAUTH2", []byte(authStr), nil
}

func (a *xoauth2SMTPAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// On error, the server sends a challenge
		// We respond with an empty response to receive the error
		return nil, nil
	}
	return nil, nil
}

// encodeSubject encodes the subject for non-ASCII characters
func encodeSubject(subject string) string {
	// Check if ASCII-only
	isASCII := true
	for _, r := range subject {
		if r > 127 {
			isASCII = false
			break
		}
	}

	if isASCII {
		return subject
	}

	// UTF-8 Base64 Encoding (RFC 2047)
	return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
}

// ReplyOptions contains options for replying to an email
type ReplyOptions struct {
	OriginalMessageID string
	OriginalFrom      string
	OriginalTo        []string
	OriginalCc        []string
	OriginalSubject   string
	OriginalDate      time.Time
	OriginalBody      string
	Body              string
	ReplyAll          bool
}

// Reply sends a reply to an email
func (c *SMTPClient) Reply(accessToken string, opts ReplyOptions) error {
	// Build reply subject
	subject := opts.OriginalSubject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	// Determine recipients
	to := []string{opts.OriginalFrom}
	var cc []string

	if opts.ReplyAll {
		// Add original To recipients (except ourselves)
		for _, addr := range opts.OriginalTo {
			email := ParseEmail(addr)
			if email != c.email {
				to = append(to, addr)
			}
		}
		// Add original CC recipients
		for _, addr := range opts.OriginalCc {
			email := ParseEmail(addr)
			if email != c.email {
				cc = append(cc, addr)
			}
		}
	}

	// Build quoted original message
	quotedBody := buildQuotedReply(opts)

	// Build full body
	fullBody := opts.Body + "\n\n" + quotedBody

	// Send using existing Send method with threading headers
	return c.sendWithHeaders(accessToken, SendOptions{
		To:      to,
		Cc:      cc,
		Subject: subject,
		Body:    fullBody,
	}, map[string]string{
		"In-Reply-To": opts.OriginalMessageID,
		"References":  opts.OriginalMessageID,
	})
}

// ForwardOptions contains options for forwarding an email
type ForwardOptions struct {
	OriginalFrom    string
	OriginalTo      []string
	OriginalSubject string
	OriginalDate    time.Time
	OriginalBody    string
	To              []string
	Body            string
	Attachments     []string
}

// Forward forwards an email to new recipients
func (c *SMTPClient) Forward(accessToken string, opts ForwardOptions) error {
	// Build forward subject
	subject := opts.OriginalSubject
	if !strings.HasPrefix(strings.ToLower(subject), "fwd:") && !strings.HasPrefix(strings.ToLower(subject), "fw:") {
		subject = "Fwd: " + subject
	}

	// Build forwarded message body
	forwardedBody := buildForwardedMessage(opts)

	// Build full body
	fullBody := opts.Body
	if fullBody != "" {
		fullBody += "\n\n"
	}
	fullBody += forwardedBody

	// Send
	return c.Send(accessToken, SendOptions{
		To:          opts.To,
		Subject:     subject,
		Body:        fullBody,
		Attachments: opts.Attachments,
	})
}

// sendWithHeaders sends an email with additional headers
func (c *SMTPClient) sendWithHeaders(accessToken string, opts SendOptions, headers map[string]string) error {
	// Establish connection
	addr := fmt.Sprintf("%s:%d", c.server, c.port)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, c.server)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Send EHLO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}

	// STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: c.server}
		if err := client.StartTLS(config); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	// XOAUTH2 authentication
	auth := &xoauth2SMTPAuth{
		email: c.email,
		token: accessToken,
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender
	if err := client.Mail(c.email); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}

	// Set recipients
	allRecipients := append(append(opts.To, opts.Cc...), opts.Bcc...)
	for _, rcpt := range allRecipients {
		email := ParseEmail(rcpt)
		if err := client.Rcpt(email); err != nil {
			return fmt.Errorf("RCPT TO failed for %s: %w", email, err)
		}
	}

	// Send email content
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}

	msg, err := c.buildMessageWithHeaders(opts, headers)
	if err != nil {
		wc.Close()
		return fmt.Errorf("failed to build message: %w", err)
	}

	if _, err := wc.Write(msg); err != nil {
		wc.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}

// buildMessageWithHeaders creates the email message with additional headers
func (c *SMTPClient) buildMessageWithHeaders(opts SendOptions, headers map[string]string) ([]byte, error) {
	var buf bytes.Buffer

	// Standard headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", c.email))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(opts.To, ", ")))

	if len(opts.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(opts.Cc, ", ")))
	}

	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeSubject(opts.Subject)))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))

	// Additional headers (e.g., In-Reply-To, References)
	for key, value := range headers {
		if value != "" {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	buf.WriteString("MIME-Version: 1.0\r\n")

	// Simple message without attachments
	contentType := "text/plain; charset=utf-8"
	if opts.HTML {
		contentType = "text/html; charset=utf-8"
	}
	buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(opts.Body)

	return buf.Bytes(), nil
}

// buildQuotedReply creates a quoted reply body
func buildQuotedReply(opts ReplyOptions) string {
	var buf bytes.Buffer

	// Add attribution line
	dateStr := opts.OriginalDate.Format("Mon, 2 Jan 2006 at 15:04")
	buf.WriteString(fmt.Sprintf("On %s, %s wrote:\n", dateStr, opts.OriginalFrom))

	// Quote original body
	lines := strings.Split(opts.OriginalBody, "\n")
	for _, line := range lines {
		buf.WriteString("> " + line + "\n")
	}

	return buf.String()
}

// buildForwardedMessage creates a forwarded message body
func buildForwardedMessage(opts ForwardOptions) string {
	var buf bytes.Buffer

	buf.WriteString("---------- Forwarded message ---------\n")
	buf.WriteString(fmt.Sprintf("From: %s\n", opts.OriginalFrom))
	buf.WriteString(fmt.Sprintf("Date: %s\n", opts.OriginalDate.Format("Mon, 2 Jan 2006 at 15:04")))
	buf.WriteString(fmt.Sprintf("Subject: %s\n", opts.OriginalSubject))
	buf.WriteString(fmt.Sprintf("To: %s\n", strings.Join(opts.OriginalTo, ", ")))
	buf.WriteString("\n")
	buf.WriteString(opts.OriginalBody)

	return buf.String()
}
