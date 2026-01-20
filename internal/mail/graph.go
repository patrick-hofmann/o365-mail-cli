package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	GraphAPIBaseURL = "https://graph.microsoft.com/v1.0"
)

// GraphClient for Microsoft Graph API operations
type GraphClient struct {
	httpClient  *http.Client
	accessToken string
}

// NewGraphClient creates a new Graph API client
func NewGraphClient(accessToken string) *GraphClient {
	return &GraphClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		accessToken: accessToken,
	}
}

// Email represents an email message
type Email struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	Subject   string    `json:"subject"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Cc        []string  `json:"cc,omitempty"`
	Date      time.Time `json:"date"`
	Body      string    `json:"body,omitempty"`
	Preview   string    `json:"preview,omitempty"`
	Unread    bool      `json:"unread"`
}

// Attachment represents an email attachment
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	SavedPath   string `json:"saved_path,omitempty"`
}

// SendOptions contains options for sending an email
type SendOptions struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Body    string
	HTML    bool
}


// GraphMessageResponse represents a message from Graph API
type GraphMessageResponse struct {
	ID                 string                `json:"id"`
	Subject            string                `json:"subject"`
	BodyPreview        string                `json:"bodyPreview"`
	Body               GraphBodyResponse     `json:"body"`
	ReceivedDateTime   string                `json:"receivedDateTime"`
	IsRead             bool                  `json:"isRead"`
	From               *GraphEmailAddressWrapper `json:"from"`
	ToRecipients       []GraphEmailAddressWrapper `json:"toRecipients"`
	CcRecipients       []GraphEmailAddressWrapper `json:"ccRecipients"`
	HasAttachments     bool                  `json:"hasAttachments"`
	InternetMessageId  string                `json:"internetMessageId"`
	ParentFolderId     string                `json:"parentFolderId"`
}

type GraphBodyResponse struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type GraphEmailAddressWrapper struct {
	EmailAddress GraphEmailAddress `json:"emailAddress"`
}

// GraphMessagesResponse represents the list response
type GraphMessagesResponse struct {
	Value    []GraphMessageResponse `json:"value"`
	NextLink string                 `json:"@odata.nextLink"`
}

// GraphFolderResponse represents a mail folder
type GraphFolderResponse struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ParentFolderId   string `json:"parentFolderId"`
	ChildFolderCount int    `json:"childFolderCount"`
	UnreadItemCount  int    `json:"unreadItemCount"`
	TotalItemCount   int    `json:"totalItemCount"`
}

// GraphFoldersResponse represents the folders list response
type GraphFoldersResponse struct {
	Value    []GraphFolderResponse `json:"value"`
	NextLink string                `json:"@odata.nextLink"`
}

// GraphAttachmentResponse represents an attachment
type GraphAttachmentResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContentType   string `json:"contentType"`
	Size          int    `json:"size"`
	ContentBytes  string `json:"contentBytes"`
}

// GraphAttachmentsResponse represents the attachments list response
type GraphAttachmentsResponse struct {
	Value []GraphAttachmentResponse `json:"value"`
}

// Folder represents a mail folder
type Folder struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	UnreadCount     int      `json:"unread_count"`
	TotalCount      int      `json:"total_count"`
	ChildFolderCount int     `json:"child_folder_count"`
}

// ListEmails lists emails from a folder
func (c *GraphClient) ListEmails(folderID string, limit int, unreadOnly bool) ([]Email, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages", GraphAPIBaseURL, url.PathEscape(folderID))

	// Build query parameters
	params := url.Values{}
	params.Set("$top", fmt.Sprintf("%d", limit))
	params.Set("$orderby", "receivedDateTime desc")
	params.Set("$select", "id,subject,bodyPreview,receivedDateTime,isRead,from,toRecipients,hasAttachments,internetMessageId")

	if unreadOnly {
		params.Set("$filter", "isRead eq false")
	}

	endpoint += "?" + params.Encode()

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result GraphMessagesResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	emails := make([]Email, len(result.Value))
	for i, msg := range result.Value {
		emails[i] = graphMessageToEmail(msg)
	}

	return emails, nil
}

// GetEmail fetches a single email with full body
func (c *GraphClient) GetEmail(folderID string, messageID string) (*Email, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages/%s", GraphAPIBaseURL, url.PathEscape(folderID), messageID)
	params := url.Values{}
	params.Set("$select", "id,subject,body,receivedDateTime,isRead,from,toRecipients,ccRecipients,hasAttachments,internetMessageId")
	endpoint += "?" + params.Encode()

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var msg GraphMessageResponse
	if err := json.Unmarshal(resp, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	email := graphMessageToEmail(msg)
	email.Body = msg.Body.Content

	return &email, nil
}

// MarkAsRead marks an email as read
func (c *GraphClient) MarkAsRead(folderID string, messageID string) error {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages/%s", GraphAPIBaseURL, url.PathEscape(folderID), messageID)
	body := map[string]interface{}{"isRead": true}

	jsonBody, _ := json.Marshal(body)
	_, err := c.doRequest("PATCH", endpoint, jsonBody)
	return err
}

// MarkAsUnread marks an email as unread
func (c *GraphClient) MarkAsUnread(folderID string, messageID string) error {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages/%s", GraphAPIBaseURL, url.PathEscape(folderID), messageID)
	body := map[string]interface{}{"isRead": false}

	jsonBody, _ := json.Marshal(body)
	_, err := c.doRequest("PATCH", endpoint, jsonBody)
	return err
}

// MoveEmail moves an email to another folder
func (c *GraphClient) MoveEmail(folderID string, messageID string, destinationFolderID string) error {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages/%s/move", GraphAPIBaseURL, url.PathEscape(folderID), messageID)
	body := map[string]string{"destinationId": destinationFolderID}

	jsonBody, _ := json.Marshal(body)
	_, err := c.doRequest("POST", endpoint, jsonBody)
	return err
}

// TrashEmail moves an email to the deleted items folder
func (c *GraphClient) TrashEmail(folderID string, messageID string) error {
	return c.MoveEmail(folderID, messageID, "deleteditems")
}

// ListEmailsFromSenders lists all emails from specific sender addresses (exact match)
// It handles pagination to return all matching emails
// Due to Graph API limitations on complex filters, this fetches all emails and filters in code
func (c *GraphClient) ListEmailsFromSenders(folderID string, senderAddresses []string, limit int) ([]Email, error) {
	if len(senderAddresses) == 0 {
		return nil, fmt.Errorf("at least one sender address required")
	}

	// Normalize addresses to lowercase for comparison
	normalizedAddrs := make(map[string]bool)
	for _, addr := range senderAddresses {
		normalizedAddrs[strings.ToLower(addr)] = true
	}

	var allEmails []Email
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages", GraphAPIBaseURL, url.PathEscape(folderID))

	params := url.Values{}
	params.Set("$top", "100") // Fetch in batches of 100
	params.Set("$orderby", "receivedDateTime desc")
	params.Set("$select", "id,subject,bodyPreview,receivedDateTime,isRead,from,toRecipients,hasAttachments,internetMessageId")

	currentEndpoint := endpoint + "?" + params.Encode()

	for currentEndpoint != "" {
		resp, err := c.doRequest("GET", currentEndpoint, nil)
		if err != nil {
			return nil, err
		}

		var result GraphMessagesResponse
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, msg := range result.Value {
			// Exact match check: extract email address and compare
			if msg.From != nil {
				fromAddr := strings.ToLower(msg.From.EmailAddress.Address)
				if normalizedAddrs[fromAddr] {
					allEmails = append(allEmails, graphMessageToEmail(msg))
					if limit > 0 && len(allEmails) >= limit {
						return allEmails, nil
					}
				}
			}
		}

		currentEndpoint = result.NextLink
	}

	return allEmails, nil
}

// SearchEmails searches emails by criteria
func (c *GraphClient) SearchEmails(folderID string, from, subject string, since time.Time, limit int) ([]Email, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages", GraphAPIBaseURL, url.PathEscape(folderID))

	params := url.Values{}
	params.Set("$top", fmt.Sprintf("%d", limit))
	params.Set("$orderby", "receivedDateTime desc")
	params.Set("$select", "id,subject,bodyPreview,receivedDateTime,isRead,from,toRecipients,hasAttachments,internetMessageId")

	// Build filter
	var filters []string
	if from != "" {
		filters = append(filters, fmt.Sprintf("contains(from/emailAddress/address,'%s')", from))
	}
	if subject != "" {
		filters = append(filters, fmt.Sprintf("contains(subject,'%s')", subject))
	}
	if !since.IsZero() {
		filters = append(filters, fmt.Sprintf("receivedDateTime ge %s", since.Format(time.RFC3339)))
	}

	if len(filters) > 0 {
		params.Set("$filter", strings.Join(filters, " and "))
	}

	endpoint += "?" + params.Encode()

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result GraphMessagesResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	emails := make([]Email, len(result.Value))
	for i, msg := range result.Value {
		emails[i] = graphMessageToEmail(msg)
	}

	return emails, nil
}

// GetAttachments downloads attachments from an email
func (c *GraphClient) GetAttachments(folderID string, messageID string, saveDir string) ([]Attachment, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/messages/%s/attachments", GraphAPIBaseURL, url.PathEscape(folderID), messageID)

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result GraphAttachmentsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var attachments []Attachment
	for _, att := range result.Value {
		attachment := Attachment{
			Filename:    att.Name,
			ContentType: att.ContentType,
			Size:        att.Size,
		}

		if saveDir != "" && att.ContentBytes != "" {
			if err := os.MkdirAll(saveDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}

			content, err := base64.StdEncoding.DecodeString(att.ContentBytes)
			if err != nil {
				continue
			}

			savePath := filepath.Join(saveDir, att.Name)
			if err := os.WriteFile(savePath, content, 0644); err != nil {
				return nil, fmt.Errorf("failed to save attachment: %w", err)
			}
			attachment.SavedPath = savePath
		}

		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

// ListFolders lists all mail folders
func (c *GraphClient) ListFolders() ([]Folder, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders?$top=100", GraphAPIBaseURL)

	var allFolders []Folder

	for endpoint != "" {
		resp, err := c.doRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}

		var result GraphFoldersResponse
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, f := range result.Value {
			allFolders = append(allFolders, Folder{
				ID:              f.ID,
				Name:            f.DisplayName,
				UnreadCount:     f.UnreadItemCount,
				TotalCount:      f.TotalItemCount,
				ChildFolderCount: f.ChildFolderCount,
			})

			// Fetch child folders if any
			if f.ChildFolderCount > 0 {
				children, err := c.listChildFolders(f.ID, f.DisplayName)
				if err == nil {
					allFolders = append(allFolders, children...)
				}
			}
		}

		endpoint = result.NextLink
	}

	return allFolders, nil
}

// listChildFolders recursively lists child folders
func (c *GraphClient) listChildFolders(parentID, parentPath string) ([]Folder, error) {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s/childFolders", GraphAPIBaseURL, parentID)

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result GraphFoldersResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	var folders []Folder
	for _, f := range result.Value {
		fullPath := parentPath + "/" + f.DisplayName
		folders = append(folders, Folder{
			ID:              f.ID,
			Name:            fullPath,
			UnreadCount:     f.UnreadItemCount,
			TotalCount:      f.TotalItemCount,
			ChildFolderCount: f.ChildFolderCount,
		})

		if f.ChildFolderCount > 0 {
			children, err := c.listChildFolders(f.ID, fullPath)
			if err == nil {
				folders = append(folders, children...)
			}
		}
	}

	return folders, nil
}

// GetFolderByName finds a folder by name and returns its ID
func (c *GraphClient) GetFolderByName(name string) (string, error) {
	// Well-known folder names that can be used directly
	wellKnown := map[string]string{
		"inbox":        "inbox",
		"drafts":       "drafts",
		"sentitems":    "sentitems",
		"deleteditems": "deleteditems",
		"junkemail":    "junkemail",
		"archive":      "archive",
	}

	lower := strings.ToLower(name)
	if id, ok := wellKnown[lower]; ok {
		return id, nil
	}

	// Search in all folders
	folders, err := c.ListFolders()
	if err != nil {
		return "", err
	}

	for _, f := range folders {
		if strings.EqualFold(f.Name, name) {
			return f.ID, nil
		}
	}

	return "", fmt.Errorf("folder '%s' not found", name)
}

// CreateFolder creates a new mail folder
func (c *GraphClient) CreateFolder(name string, parentFolderID string) error {
	var endpoint string
	if parentFolderID != "" {
		endpoint = fmt.Sprintf("%s/me/mailFolders/%s/childFolders", GraphAPIBaseURL, parentFolderID)
	} else {
		endpoint = fmt.Sprintf("%s/me/mailFolders", GraphAPIBaseURL)
	}

	body := map[string]string{"displayName": name}
	jsonBody, _ := json.Marshal(body)

	_, err := c.doRequest("POST", endpoint, jsonBody)
	return err
}

// DeleteFolder deletes a mail folder
func (c *GraphClient) DeleteFolder(folderID string) error {
	endpoint := fmt.Sprintf("%s/me/mailFolders/%s", GraphAPIBaseURL, folderID)
	_, err := c.doRequest("DELETE", endpoint, nil)
	return err
}

// Send sends an email
func (c *GraphClient) Send(opts SendOptions) error {
	toRecipients := make([]GraphEmailAddressWrapper, len(opts.To))
	for i, to := range opts.To {
		toRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(to)},
		}
	}

	ccRecipients := make([]GraphEmailAddressWrapper, len(opts.Cc))
	for i, cc := range opts.Cc {
		ccRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(cc)},
		}
	}

	bccRecipients := make([]GraphEmailAddressWrapper, len(opts.Bcc))
	for i, bcc := range opts.Bcc {
		bccRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(bcc)},
		}
	}

	contentType := "text"
	if opts.HTML {
		contentType = "html"
	}

	message := map[string]interface{}{
		"subject": opts.Subject,
		"body": map[string]string{
			"contentType": contentType,
			"content":     opts.Body,
		},
		"toRecipients": toRecipients,
	}

	if len(ccRecipients) > 0 {
		message["ccRecipients"] = ccRecipients
	}
	if len(bccRecipients) > 0 {
		message["bccRecipients"] = bccRecipients
	}

	request := map[string]interface{}{
		"message":         message,
		"saveToSentItems": true,
	}

	jsonBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	_, err = c.doRequest("POST", GraphAPIBaseURL+"/me/sendMail", jsonBody)
	return err
}

// Reply sends a reply using native Graph API
func (c *GraphClient) Reply(messageID string, comment string, replyAll bool) error {
	action := "reply"
	if replyAll {
		action = "replyAll"
	}
	endpoint := fmt.Sprintf("%s/me/messages/%s/%s", GraphAPIBaseURL, messageID, action)

	body := map[string]interface{}{}
	if comment != "" {
		body["comment"] = comment
	}

	jsonBody, _ := json.Marshal(body)
	_, err := c.doRequest("POST", endpoint, jsonBody)
	return err
}

// Forward forwards an email using native Graph API
func (c *GraphClient) Forward(messageID string, to []string, comment string) error {
	endpoint := fmt.Sprintf("%s/me/messages/%s/forward", GraphAPIBaseURL, messageID)

	toRecipients := make([]GraphEmailAddressWrapper, len(to))
	for i, addr := range to {
		toRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(addr)},
		}
	}

	body := map[string]interface{}{
		"toRecipients": toRecipients,
	}
	if comment != "" {
		body["comment"] = comment
	}

	jsonBody, _ := json.Marshal(body)
	_, err := c.doRequest("POST", endpoint, jsonBody)
	return err
}

// SaveDraft saves an email as draft and returns the draft ID
func (c *GraphClient) SaveDraft(to, cc []string, subject, body string, html bool) (string, error) {
	toRecipients := make([]GraphEmailAddressWrapper, len(to))
	for i, addr := range to {
		toRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(addr)},
		}
	}

	ccRecipients := make([]GraphEmailAddressWrapper, len(cc))
	for i, addr := range cc {
		ccRecipients[i] = GraphEmailAddressWrapper{
			EmailAddress: GraphEmailAddress{Address: ParseEmail(addr)},
		}
	}

	contentType := "text"
	if html {
		contentType = "html"
	}

	message := map[string]interface{}{
		"subject": subject,
		"body": map[string]string{
			"contentType": contentType,
			"content":     body,
		},
		"toRecipients": toRecipients,
	}

	if len(ccRecipients) > 0 {
		message["ccRecipients"] = ccRecipients
	}

	jsonBody, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest("POST", GraphAPIBaseURL+"/me/messages", jsonBody)
	if err != nil {
		return "", err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.ID, nil
}

// ListDrafts lists draft emails
func (c *GraphClient) ListDrafts(limit int) ([]Email, error) {
	return c.ListEmails("drafts", limit, false)
}

// SendDraft sends a draft and deletes it
func (c *GraphClient) SendDraft(messageID string) error {
	endpoint := fmt.Sprintf("%s/me/messages/%s/send", GraphAPIBaseURL, messageID)
	_, err := c.doRequest("POST", endpoint, nil)
	return err
}

// DeleteDraft deletes a draft
func (c *GraphClient) DeleteDraft(messageID string) error {
	endpoint := fmt.Sprintf("%s/me/messages/%s", GraphAPIBaseURL, messageID)
	_, err := c.doRequest("DELETE", endpoint, nil)
	return err
}

// doRequest performs an HTTP request to Graph API
func (c *GraphClient) doRequest(method, endpoint string, body []byte) ([]byte, error) {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, endpoint, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, endpoint, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Graph API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// graphMessageToEmail converts a Graph API message to our Email struct
func graphMessageToEmail(msg GraphMessageResponse) Email {
	email := Email{
		MessageID: msg.ID,
		Subject:   msg.Subject,
		Preview:   msg.BodyPreview,
		Unread:    !msg.IsRead,
	}

	if t, err := time.Parse(time.RFC3339, msg.ReceivedDateTime); err == nil {
		email.Date = t
	}

	if msg.From != nil {
		email.From = formatGraphAddress(msg.From.EmailAddress)
	}

	for _, to := range msg.ToRecipients {
		email.To = append(email.To, formatGraphAddress(to.EmailAddress))
	}

	return email
}

// formatGraphAddress formats a Graph API email address
func formatGraphAddress(addr GraphEmailAddress) string {
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", addr.Name, addr.Address)
	}
	return addr.Address
}

// GraphMessage for sending
type GraphMessage struct {
	Subject       string                   `json:"subject"`
	Body          GraphBody                `json:"body"`
	ToRecipients  []GraphEmailAddressWrapper `json:"toRecipients"`
	CcRecipients  []GraphEmailAddressWrapper `json:"ccRecipients,omitempty"`
	BccRecipients []GraphEmailAddressWrapper `json:"bccRecipients,omitempty"`
}

// GraphBody represents the email body
type GraphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// GraphEmailAddress represents an email address
type GraphEmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// ParseEmail extracts an email address from a string like "Name <email@example.com>"
func ParseEmail(addr string) string {
	addr = strings.TrimSpace(addr)
	if idx := strings.Index(addr, "<"); idx != -1 {
		if end := strings.Index(addr, ">"); end != -1 {
			return addr[idx+1 : end]
		}
	}
	return addr
}

