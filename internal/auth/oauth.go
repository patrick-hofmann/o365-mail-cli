package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

const (
	// DefaultClientID is the public client ID of the o365-mail-cli Azure App
	DefaultClientID = "5aa6d895-1072-41c4-beb6-d8e3fdf0e7cd"

	// Authority is Microsoft's common tenant for multi-tenant apps
	Authority = "https://login.microsoftonline.com/common"
)

// Scopes for IMAP and SMTP access
var Scopes = []string{
	"https://outlook.office.com/IMAP.AccessAsUser.All",
	"https://outlook.office.com/SMTP.Send",
	// offline_access is automatically requested by MSAL
}

// OAuthClient manages OAuth2 authentication
type OAuthClient struct {
	clientID   string
	app        public.Client
	tokenCache *TokenCache
	email      string
}

// DeviceCodeResult contains info for the Device Code Flow
type DeviceCodeResult struct {
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Message         string
}

// NewOAuthClient creates a new OAuth client
func NewOAuthClient(clientID string, cacheDir string) (*OAuthClient, error) {
	if clientID == "" {
		clientID = DefaultClientID
	}

	cache := NewTokenCache(cacheDir)

	app, err := public.New(clientID,
		public.WithAuthority(Authority),
		public.WithCache(cache),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MSAL app: %w", err)
	}

	return &OAuthClient{
		clientID:   clientID,
		app:        app,
		tokenCache: cache,
	}, nil
}

// SetEmail sets the email address for account hints
func (c *OAuthClient) SetEmail(email string) {
	c.email = email
}

// GetAccessToken retrieves a valid access token for a specific account
// Tries cache first, then refresh, then requires new login
func (c *OAuthClient) GetAccessToken(ctx context.Context, email string) (string, error) {
	// Try to get a token from cache first
	accounts, err := c.app.Accounts(ctx)
	if err == nil && len(accounts) > 0 {
		// Search for specific account
		for _, account := range accounts {
			if account.PreferredUsername == email {
				result, err := c.app.AcquireTokenSilent(ctx, Scopes,
					public.WithSilentAccount(account),
				)
				if err == nil {
					return result.AccessToken, nil
				}
				// Silent acquisition failed for this account
				return "", fmt.Errorf("token expired for %s, please run 'auth login' again", email)
			}
		}
		return "", fmt.Errorf("no token found for %s, please run 'auth login' first", email)
	}

	return "", fmt.Errorf("no valid token found, please run 'auth login' first")
}

// ListAccounts returns all accounts in the token cache
func (c *OAuthClient) ListAccounts(ctx context.Context) ([]string, error) {
	accounts, err := c.app.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	emails := make([]string, 0, len(accounts))
	for _, account := range accounts {
		emails = append(emails, account.PreferredUsername)
	}
	return emails, nil
}

// StartDeviceCodeFlow initiates the Device Code Flow
// Returns the device code that the user must enter in the browser
func (c *OAuthClient) StartDeviceCodeFlow(ctx context.Context) (*DeviceCodeResult, <-chan AuthResult, error) {
	resultChan := make(chan AuthResult, 1)

	// Start device code flow - returns the code immediately
	deviceCode, err := c.app.AcquireTokenByDeviceCode(ctx, Scopes)
	if err != nil {
		close(resultChan)
		return nil, nil, fmt.Errorf("failed to start device code flow: %w", err)
	}

	// Wait for token in background
	go func() {
		defer close(resultChan)
		result, err := deviceCode.AuthenticationResult(ctx)
		if err != nil {
			resultChan <- AuthResult{Error: err}
			return
		}

		// Save cache
		if err := c.tokenCache.Save(); err != nil {
			resultChan <- AuthResult{Error: fmt.Errorf("failed to save token: %w", err)}
			return
		}

		resultChan <- AuthResult{
			AccessToken: result.AccessToken,
			Email:       result.Account.PreferredUsername,
			ExpiresAt:   result.ExpiresOn,
		}
	}()

	return &DeviceCodeResult{
		UserCode:        deviceCode.Result.UserCode,
		VerificationURL: deviceCode.Result.VerificationURL,
		ExpiresIn:       int(time.Until(deviceCode.Result.ExpiresOn).Seconds()),
		Message:         deviceCode.Result.Message,
	}, resultChan, nil
}

// AuthResult contains the result of an authentication
type AuthResult struct {
	AccessToken string
	Email       string
	ExpiresAt   time.Time
	Error       error
}

// Logout removes a specific account from the token cache
func (c *OAuthClient) Logout(ctx context.Context, email string) error {
	accounts, err := c.app.Accounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}

	for _, account := range accounts {
		if account.PreferredUsername == email {
			if err := c.app.RemoveAccount(ctx, account); err != nil {
				return fmt.Errorf("failed to remove account: %w", err)
			}
			// Save token cache
			return c.tokenCache.Save()
		}
	}

	return fmt.Errorf("account %s not found", email)
}

// LogoutAll removes all stored tokens
func (c *OAuthClient) LogoutAll(ctx context.Context) error {
	accounts, err := c.app.Accounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}

	for _, account := range accounts {
		if err := c.app.RemoveAccount(ctx, account); err != nil {
			return fmt.Errorf("failed to remove account: %w", err)
		}
	}

	return c.tokenCache.Clear()
}

// GetStatus returns the current auth status for a specific account
func (c *OAuthClient) GetStatus(ctx context.Context, email string) (*AuthStatus, error) {
	accounts, err := c.app.Accounts(ctx)
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		return &AuthStatus{LoggedIn: false}, nil
	}

	// Search for specific account
	for _, account := range accounts {
		if email == "" || account.PreferredUsername == email {
			// Try to get token to check expiry
			result, err := c.app.AcquireTokenSilent(ctx, Scopes,
				public.WithSilentAccount(account),
			)
			if err != nil {
				return &AuthStatus{
					LoggedIn:     true,
					Email:        account.PreferredUsername,
					TokenExpired: true,
				}, nil
			}

			return &AuthStatus{
				LoggedIn:     true,
				Email:        result.Account.PreferredUsername,
				TokenExpired: false,
				ExpiresAt:    result.ExpiresOn,
			}, nil
		}
	}

	return &AuthStatus{LoggedIn: false}, nil
}

// GetAllStatuses returns the auth status for all accounts
func (c *OAuthClient) GetAllStatuses(ctx context.Context) ([]*AuthStatus, error) {
	accounts, err := c.app.Accounts(ctx)
	if err != nil {
		return nil, err
	}

	statuses := make([]*AuthStatus, 0, len(accounts))
	for _, account := range accounts {
		result, err := c.app.AcquireTokenSilent(ctx, Scopes,
			public.WithSilentAccount(account),
		)
		if err != nil {
			statuses = append(statuses, &AuthStatus{
				LoggedIn:     true,
				Email:        account.PreferredUsername,
				TokenExpired: true,
			})
		} else {
			statuses = append(statuses, &AuthStatus{
				LoggedIn:     true,
				Email:        result.Account.PreferredUsername,
				TokenExpired: false,
				ExpiresAt:    result.ExpiresOn,
			})
		}
	}

	return statuses, nil
}

// AuthStatus contains the auth status
type AuthStatus struct {
	LoggedIn     bool
	Email        string
	TokenExpired bool
	ExpiresAt    time.Time
}

// GenerateXOAuth2String creates the XOAUTH2 auth string for IMAP/SMTP
func GenerateXOAuth2String(email, accessToken string) string {
	authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", email, accessToken)
	return base64.StdEncoding.EncodeToString([]byte(authStr))
}
