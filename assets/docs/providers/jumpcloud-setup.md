# JumpCloud Setup Guide for Amazon Bedrock Integration

This guide walks you through setting up JumpCloud as an identity provider for the distribution landing page with OIDC authentication.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Create an SSO Application](#2-create-an-sso-application)
3. [Configure the Application](#3-configure-the-application)
4. [Collect Required Information](#4-collect-required-information)
5. [Store the Client Secret](#5-store-the-client-secret)
6. [Test the Setup](#6-test-the-setup)

---

## 1. Prerequisites

- A JumpCloud administrator account
- Access to the JumpCloud Admin Console
- AWS CLI configured with appropriate permissions
- The landing page custom domain name (e.g., `downloads.example.com`)

---

## 2. Create an SSO Application

### Step 2.1: Navigate to SSO Applications

1. Log in to the JumpCloud Admin Console at `https://console.jumpcloud.com`
2. Go to **SSO Applications** in the left navigation
3. Click **+ Add New Application**

### Step 2.2: Select Application Type

1. Search for **Custom OIDC (SSO)** or select it from the list
2. Click **Configure**

### Step 2.3: Name the Application

1. Set **Display Label** to `Claude Code Distribution` (or your preferred name)
2. Optionally add a description
3. Click **Next**

---

## 3. Configure the Application

### Step 3.1: Configure SSO Settings

1. Under **SSO** tab, configure:
   - **Redirect URIs**: `https://<your-custom-domain>/oauth2/idpresponse`
     - Replace `<your-custom-domain>` with your landing page domain (e.g., `downloads.example.com`)
   - **Login URL**: `https://<your-custom-domain>`
2. Under **Client Authentication Type**, select **Client Secret Post** or **Client Secret Basic**

### Step 3.2: Configure Scopes

Ensure the following scopes are enabled:
- `openid`
- `profile`
- `email`

### Step 3.3: Assign User Groups

1. Go to the **User Groups** tab
2. Select the groups that should have access to the distribution landing page
3. Click **Save**

### Step 3.4: Activate the Application

1. Click **Activate** to enable the application
2. The application is now ready for use

---

## 4. Collect Required Information

After creating the application, collect these values:

| Parameter | Value | Notes |
|-----------|-------|-------|
| **Domain** | `oauth.id.jumpcloud.com` | Always the same for all JumpCloud tenants |
| **Client ID** | Your application's Client ID | Found in the SSO application settings (UUID format) |
| **Client Secret** | Your application's Client Secret | Found in the SSO application settings |

### Where to Find the Client ID and Secret

1. Go to **SSO Applications** in the JumpCloud Admin Console
2. Click on your application
3. Navigate to the **SSO** tab
4. The **Client ID** and **Client Secret** are displayed in the configuration section

### JumpCloud OIDC Endpoints

JumpCloud uses these standard OIDC endpoints (for reference):

| Endpoint | URL |
|----------|-----|
| Issuer | `https://oauth.id.jumpcloud.com/` |
| Authorization | `https://oauth.id.jumpcloud.com/oauth2/auth` |
| Token | `https://oauth.id.jumpcloud.com/oauth2/token` |
| UserInfo | `https://oauth.id.jumpcloud.com/userinfo` |
| Discovery | `https://oauth.id.jumpcloud.com/.well-known/openid-configuration` |

---

## 5. Store the Client Secret

The client secret must be stored in AWS Secrets Manager before deployment.

### Step 5.1: Create the Secret

```bash
aws secretsmanager create-secret \
  --name "jumpcloud/claude-code-distribution/client-secret" \
  --description "JumpCloud client secret for Claude Code distribution landing page" \
  --secret-string "<your-client-secret>" \
  --region <your-aws-region>
```

### Step 5.2: Note the Secret ARN

The command output includes the ARN. Save it for the `ccwb init` wizard:

```
arn:aws:secretsmanager:<region>:<account-id>:secret:jumpcloud/claude-code-distribution/client-secret-XXXXXX
```

---

## 6. Test the Setup

### Step 6.1: Verify OIDC Discovery

```bash
curl https://oauth.id.jumpcloud.com/.well-known/openid-configuration
```

Should return a JSON response with OIDC endpoints.

### Step 6.2: Run the Init Wizard

```bash
poetry run ccwb init
```

When prompted for distribution landing page configuration:
- **Identity provider**: Select **JumpCloud**
- **Domain**: `oauth.id.jumpcloud.com`
- **Client ID**: Your JumpCloud application Client ID
- **Client Secret ARN**: The Secrets Manager ARN from Step 5

### Step 6.3: Deploy

```bash
poetry run ccwb deploy distribution
```

### Step 6.4: Verify Authentication

1. Navigate to your landing page URL (e.g., `https://downloads.example.com`)
2. You should be redirected to JumpCloud for authentication
3. After signing in, you should see the distribution landing page

---

## Troubleshooting

### "Invalid redirect URI" Error

- Ensure the redirect URI in JumpCloud exactly matches: `https://<your-domain>/oauth2/idpresponse`
- No trailing slashes
- Must use HTTPS

### Authentication Fails After Redirect

- Verify the Client ID and Client Secret are correct
- Check that the secret in AWS Secrets Manager matches the JumpCloud client secret
- Ensure the user is assigned to the application (directly or via group)

### "Access Denied" After Login

- Verify the user is in a group assigned to the JumpCloud SSO application
- Check JumpCloud Admin Console > **Directory Insights** for authentication events

### Token or Scope Issues

- Ensure `openid`, `profile`, and `email` scopes are enabled on the application
- Verify the client authentication type matches what the ALB expects

---

## Security Best Practices

1. **User Access Control**:
   - Use JumpCloud user groups to control access to the application
   - Regularly review group membership
   - Remove access promptly when users leave the organization

2. **MFA**:
   - Enable MFA policies in JumpCloud for additional security
   - JumpCloud supports TOTP, WebAuthn, and push-based MFA

3. **Secret Management**:
   - Rotate the client secret periodically
   - Update the Secrets Manager value and redeploy when rotating
   - Use AWS Secrets Manager automatic rotation if possible

4. **Monitoring**:
   - Monitor JumpCloud Directory Insights for failed authentication attempts
   - Set up alerts for unusual access patterns
