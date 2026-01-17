# O365 Mail CLI

A cross-platform CLI tool for Office 365 email access via OAuth2 – no admin approval, no API keys required.

## How It Works

The tool uses the **OAuth2 Device Authorization Flow** with a Multi-Tenant Public Client App. Any O365 user can authenticate without requiring administrator approval.

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   CLI Tool  │────▶│  Microsoft Login │────▶│  Office 365     │
│             │     │  (Device Code)   │     │  IMAP/SMTP      │
└─────────────┘     └──────────────────┘     └─────────────────┘
       │                    │                        │
       │  1. Device Code    │                        │
       │◀───────────────────│                        │
       │                    │                        │
       │  2. User opens     │                        │
       │     browser &      │                        │
       │     enters code    │                        │
       │                    │                        │
       │  3. Access Token   │                        │
       │◀───────────────────│                        │
       │                    │                        │
       │  4. XOAUTH2 Login  │                        │
       │─────────────────────────────────────────────▶│
       │                    │                        │
       │  5. Emails         │                        │
       │◀────────────────────────────────────────────│
└─────────────┘     └──────────────────┘     └─────────────────┘
```

## Prerequisites

### Option A: Register Your Own Azure App (Recommended)

1. **Open Azure Portal**: https://portal.azure.com
2. **Create App Registration**:
   - Navigate to "Microsoft Entra ID" → "App registrations" → "New registration"
   - Name: e.g., "O365 Mail CLI"
   - Supported account types: **"Accounts in any organizational directory (Any Microsoft Entra ID tenant - Multitenant)"**
   - Redirect URI: Type "Public client/native", URI: `http://localhost`
   - Click "Register"

3. **Copy Client ID**: On the overview page, find the "Application (client) ID"

4. **Enable Public Client**:
   - "Authentication" → "Advanced settings"
   - Set "Allow public client flows" to **Yes**
   - Save

5. **Add API Permissions**:
   - "API permissions" → "Add a permission" → "Microsoft Graph"
   - Select "Delegated permissions"
   - Add:
     - `IMAP.AccessAsUser.All`
     - `SMTP.Send`
     - `offline_access` (for Refresh Tokens)
     - `User.Read` (for profile information)
   - Alternatively: "APIs my organization uses" → "Office 365 Exchange Online"
     - `IMAP.AccessAsUser.All`
     - `SMTP.Send`

6. **Done!** No admin consent required for these permissions.

### Option B: Use Existing Client ID (Gray Area)

Thunderbird and other clients use publicly known Client IDs:

```
# Thunderbird
08162f7c-0fd2-4200-a84a-f25a4db0b584

# Microsoft Office
d3590ed6-52b3-4102-aeff-aad2292ab01c
```

⚠️ These IDs work but are technically not intended for third-party tools.

## Installation

### Pre-compiled Binaries

Download the appropriate binary from [Releases](https://github.com/patrick-hofmann/o365-mail-cli/releases):

| Platform | File |
|----------|------|
| Windows | `o365-mail-cli-windows-amd64.exe` |
| macOS Intel | `o365-mail-cli-darwin-amd64` |
| macOS Apple Silicon | `o365-mail-cli-darwin-arm64` |
| Linux | `o365-mail-cli-linux-amd64` |

### Build from Source

```bash
# Go 1.21+ required
git clone https://github.com/patrick-hofmann/o365-mail-cli.git
cd o365-mail-cli
go build -o o365-mail-cli ./cmd/o365-mail-cli
```

### Cross-Compilation

```bash
# All platforms at once
make build-all

# Or manually
GOOS=windows GOARCH=amd64 go build -o dist/o365-mail-cli-windows-amd64.exe ./cmd/o365-mail-cli
GOOS=darwin GOARCH=amd64 go build -o dist/o365-mail-cli-darwin-amd64 ./cmd/o365-mail-cli
GOOS=darwin GOARCH=arm64 go build -o dist/o365-mail-cli-darwin-arm64 ./cmd/o365-mail-cli
GOOS=linux GOARCH=amd64 go build -o dist/o365-mail-cli-linux-amd64 ./cmd/o365-mail-cli
```

## Configuration

The tool works out-of-the-box with the built-in Client ID. Optionally, you can create `~/.o365-mail-cli/config.yaml`:

```yaml
# Azure App Client ID (optional, default is already configured)
client_id: "5aa6d895-1072-41c4-beb6-d8e3fdf0e7cd"

# Active account (set automatically on login)
current_account: "user@example.com"

# IMAP/SMTP Server (O365 defaults)
imap_server: "outlook.office365.com"
imap_port: 993
smtp_server: "smtp.office365.com"
smtp_port: 587
```

Or set environment variables:

```bash
export O365_CLIENT_ID="your-client-id"
export O365_ACCOUNT="user@example.com"
```

## Usage

### Authentication

```bash
# Login (multiple accounts supported)
o365-mail-cli auth login

# List all logged-in accounts
o365-mail-cli auth list

# Check token status for all accounts
o365-mail-cli auth status

# Switch active account
o365-mail-cli auth switch user2@example.com

# Logout specific account
o365-mail-cli auth logout user@example.com

# Logout all accounts
o365-mail-cli auth logout --all
```

### Multi-Account Support

```bash
# Specify account for command (overrides active account)
o365-mail-cli --account user2@example.com mail list

# Or via environment variable
O365_ACCOUNT="user2@example.com" o365-mail-cli mail list
```

Priority: `--account` flag → `O365_ACCOUNT` env → `current_account` in config

### Reading Emails

```bash
# Last 10 emails from INBOX
o365-mail-cli mail list

# Show more emails
o365-mail-cli mail list --limit 50

# Read from different folder
o365-mail-cli mail list --folder "Sent Items"

# Show email content
o365-mail-cli mail read <message-id>

# Output as JSON (for scripting)
o365-mail-cli mail list --json
```

### Sending Emails

```bash
# Simple email
o365-mail-cli mail send \
  --to "recipient@example.com" \
  --subject "Test" \
  --body "Hello World!"

# With CC and attachment
o365-mail-cli mail send \
  --to "recipient@example.com" \
  --cc "copy@example.com" \
  --subject "Document" \
  --body "See attachment" \
  --attach "/path/to/file.pdf"

# Read body from file
o365-mail-cli mail send \
  --to "recipient@example.com" \
  --subject "Report" \
  --body-file "email-body.txt"

# HTML email
o365-mail-cli mail send \
  --to "recipient@example.com" \
  --subject "Newsletter" \
  --body-file "newsletter.html" \
  --html
```

### Managing Folders

```bash
# List all folders
o365-mail-cli folders list

# Create folder
o365-mail-cli folders create "Archive/2024"

# Delete folder
o365-mail-cli folders delete "Old Folder"
```

## Token Management

The tool stores OAuth2 tokens in `~/.o365-mail-cli/token.json`:

```json
{
  "access_token": "eyJ0eXAi...",
  "refresh_token": "0.AAAA...",
  "expiry": "2024-01-15T10:30:00Z",
  "token_type": "Bearer"
}
```

- **Access Token**: Valid for ~1 hour
- **Refresh Token**: Valid for ~90 days (automatically renewed)
- The tool automatically refreshes when the access token expires

### Security

```bash
# Token file has restricted permissions (0600)
chmod 600 ~/.o365-mail-cli/token.json

# For enhanced security: store token in system keyring
o365-mail-cli config set token_storage keyring
```

## Scripting & Automation

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Authentication error |
| 3 | Network error |
| 4 | Configuration error |

### JSON Output

```bash
# All emails as JSON
o365-mail-cli mail list --json | jq '.[] | {from: .from, subject: .subject}'

# Save to file
o365-mail-cli mail list --json > emails.json
```

### Example: Count Unread Emails

```bash
#!/bin/bash
COUNT=$(o365-mail-cli mail list --folder INBOX --unread --json | jq length)
echo "You have $COUNT unread emails"
```

### Example: Daily Email Report

```bash
#!/bin/bash
o365-mail-cli mail list --since "24h" --json | \
  jq -r '.[] | "\(.date) | \(.from) | \(.subject)"' | \
  column -t -s '|'
```

## Troubleshooting

### "AADSTS700016: Application not found"

The Client ID is not registered or incorrectly configured.
→ Create your own Azure App (see Option A).

### "AADSTS65001: User has not consented"

The user must consent to the permissions.
→ Device Code Flow automatically prompts for consent.

### "AUTHENTICATE failed"

The OAuth token is invalid or expired.
→ Run `o365-mail-cli auth logout` then `o365-mail-cli auth login`.

### Token refresh fails

The refresh token has expired (after ~90 days of inactivity).
→ Run `o365-mail-cli auth login` for new login.

## Development

### Project Structure

```
o365-mail-cli/
├── cmd/
│   └── o365-mail-cli/
│       └── main.go           # Entry Point
├── internal/
│   ├── auth/
│   │   ├── oauth.go          # OAuth2 Device Flow
│   │   └── token.go          # Token Storage
│   ├── mail/
│   │   ├── imap.go           # IMAP with XOAUTH2
│   │   └── smtp.go           # SMTP with XOAUTH2
│   ├── config/
│   │   └── config.go         # Configuration
│   └── cmd/
│       ├── root.go           # CLI Root Command
│       ├── auth.go           # auth Subcommands
│       └── mail.go           # mail Subcommands
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Run Tests

```bash
go test ./...
```

### Test with Real Account

```bash
# Test mode with logging
O365_DEBUG=1 o365-mail-cli mail list
```

## License

MIT License - see LICENSE file.

## Similar Projects

- [msal-go](https://github.com/AzureAD/microsoft-authentication-library-for-go) - Microsoft's official Go Auth Library
- [go-imap](https://github.com/emersion/go-imap) - IMAP Library for Go
- [mutt_oauth2.py](https://gitlab.com/muttmua/mutt/-/blob/master/contrib/mutt_oauth2.py) - OAuth2 for Mutt (Python)
