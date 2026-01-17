# Azure App Registration - Step-by-Step Guide

This guide shows how to register your own Azure App for the o365-mail-cli tool.

## Why Your Own App?

- **Control**: You see exactly which permissions are granted
- **Security**: No dependency on third-party Client IDs
- **Compliance**: Required in some organizations
- **Branding**: Users see your app name during login

## Prerequisites

- A Microsoft account (personal or business)
- Access to [Azure Portal](https://portal.azure.com)
- No Azure subscription required (App Registrations are free)

## Step 1: Open Azure Portal

1. Go to https://portal.azure.com
2. Sign in with your Microsoft account

## Step 2: Open Microsoft Entra ID

1. Click on **"Microsoft Entra ID"** in the left menu (formerly Azure Active Directory)
2. If not visible: Search for "Entra" in the search bar at the top

## Step 3: Create App Registration

1. Click on **"App registrations"** in the left menu
2. Click on **"+ New registration"**

### Fill Out Registration Form:

**Name:**
```
O365 Mail CLI
```
(or a name of your choice)

**Supported account types:**
Select: **"Accounts in any organizational directory (Any Microsoft Entra ID tenant - Multitenant)"**

> This option is important! It allows users from any O365 organization to sign in.

**Redirect URI:**
- Type: **"Public client/native (mobile & desktop)"**
- URI: `http://localhost`

3. Click on **"Register"**

## Step 4: Copy Client ID

After registration, you'll land on your app's overview page.

1. Copy the **"Application (client) ID"** - this is your Client ID
2. Store it securely - you'll need it for configuration

Example:
```
Application (client) ID: 12345678-abcd-1234-abcd-123456789012
```

## Step 5: Enable Public Client Flow

1. Click on **"Authentication"** in the left menu
2. Scroll down to **"Advanced settings"**
3. Set **"Allow public client flows"** to **"Yes"**
4. Click on **"Save"**

> This setting is required for the Device Code Flow.

## Step 6: Add API Permissions

1. Click on **"API permissions"** in the left menu
2. Click on **"+ Add a permission"**

### Option A: Microsoft Graph Permissions (Recommended)

1. Select **"Microsoft Graph"**
2. Select **"Delegated permissions"**
3. Search for and enable:
   - `IMAP.AccessAsUser.All` - IMAP access
   - `SMTP.Send` - SMTP sending
   - `offline_access` - Refresh Token (for longer sessions)
   - `User.Read` - User info (optional, for email address)
4. Click on **"Add permissions"**

### Option B: Office 365 Exchange Online (Alternative)

1. Click on **"APIs my organization uses"**
2. Search for **"Office 365 Exchange Online"**
3. Select **"Delegated permissions"**
4. Enable:
   - `IMAP.AccessAsUser.All`
   - `SMTP.Send`
5. Click on **"Add permissions"**

### Admin Consent?

For these permissions, **no admin consent is required**.
Each user can consent themselves during their first login.

If you see "Admin consent required": Check if you selected the correct
permissions (Delegated, not Application).

## Step 7: Done!

Your app is now ready. The overview should look like this:

```
App Registration: O365 Mail CLI
├── Application (client) ID: 12345678-abcd-...
├── Directory (tenant) ID: common (Multi-tenant)
├── Supported account types: Multiple organizations
├── Authentication
│   └── Allow public client flows: Yes
└── API Permissions
    ├── IMAP.AccessAsUser.All (Delegated)
    ├── SMTP.Send (Delegated)
    ├── offline_access (Delegated)
    └── User.Read (Delegated)
```

## Step 8: Use in CLI

Configure the CLI with your Client ID:

```bash
# Via config file
o365-mail-cli config set client_id "12345678-abcd-1234-abcd-123456789012"

# Or via environment variable
export O365_CLIENT_ID="12345678-abcd-1234-abcd-123456789012"

# Or via config.yaml
echo 'client_id: "12345678-abcd-1234-abcd-123456789012"' > ~/.o365-mail-cli/config.yaml
```

Then login:

```bash
o365-mail-cli auth login
```

---

## Troubleshooting

### "Application not found in directory"

The app is not yet configured for Multi-Tenant.
→ Check Step 3: "Supported account types" must be Multi-Tenant.

### "Admin consent required"

You selected Application Permissions instead of Delegated Permissions.
→ Delete the permissions and add them as "Delegated".

### "Invalid redirect URI"

The Redirect URI is missing or incorrect.
→ Add `http://localhost` as Public client/native URI.

### "Public client flows disabled"

The Device Code Flow is not enabled.
→ Enable "Allow public client flows" under Authentication.

---

## Security Notes

1. **Client ID is semi-public**: It can be in your code, but don't share it unnecessarily
2. **No Client Secret needed**: Public Clients use PKCE, no secret required
3. **Store tokens locally**: Access/Refresh Tokens are stored locally
4. **Check regularly**: Periodically verify the app permissions in Azure Portal

## Further Reading

- [Microsoft Entra App Registration Docs](https://learn.microsoft.com/en-us/azure/active-directory/develop/quickstart-register-app)
- [OAuth 2.0 Device Code Flow](https://learn.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-device-code)
- [IMAP OAuth2 for Exchange Online](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth)
