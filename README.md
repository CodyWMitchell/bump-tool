# 💚 Bump Brat

A TUI tool for bumping frontend production hashes in `app-interface` via GitLab automation.

## What It Does

Bump Brat helps you update frontend project commit references to their latest versions. It:

- Shows all configured frontend projects with their current status
- Displays what commits will be included in each bump
- Clones your fork of `app-interface` and syncs from `service/app-interface`
- Creates a bump branch, commits, pushes, and opens a GitLab MR automatically

## Quick Start

```bash
# 1. Clone this repo
cd bump-tool

# 2. Set up your credentials (see Authentication below)
cp .env.example .env  # then edit with your values

# 3. Run the tool
make run
```

Use arrow keys to navigate, press **enter** to bump a project, press **q** to quit.

## Authentication

Set these in `.env` (copy from `.env.example`):

```bash
# JIRA — required for automatic ticket creation
# Generate at: https://id.atlassian.com/manage-profile/security/api-tokens
JIRA_URL=https://redhat.atlassian.net
JIRA_USERNAME=your-email@redhat.com
JIRA_API_TOKEN=your-jira-api-token

# GitHub — optional, but recommended to avoid low unauthenticated rate limits
# Generate at: https://github.com/settings/tokens (select scope: public_repo)
GITHUB_TOKEN=ghp_your_token_here

# GitLab — required for app-interface automation
# Generate at: https://gitlab.com/-/user_settings/personal_access_tokens
# Required scopes: api + write_repository
GITLAB_TOKEN=glpat-your_token_here
GITLAB_BASE_URL=https://gitlab.com
APP_INTERFACE_FORK_PROJECT=your-username/app-interface
APP_INTERFACE_UPSTREAM_PROJECT=service/app-interface
APP_INTERFACE_TARGET_BRANCH=master
```

### Setting up JIRA credentials

1. **Create an API token** at https://id.atlassian.com/manage-profile/security/api-tokens
   - Click "Create API token"
   - Give it a name (e.g., "bump-brat")
   - Copy the token

2. **Add to `.env`** file:
   - `JIRA_URL` (e.g., https://redhat.atlassian.net)
   - `JIRA_USERNAME` (your email)
   - `JIRA_API_TOKEN` (the token you just created)

### Setting up GitHub token

1. **Create a token** at https://github.com/settings/tokens
   - Click "Generate new token (classic)"
   - Give it a name (e.g., "bump-brat")
   - Select scope: `public_repo`
   - Generate and copy the token

2. **Add to `.env`** file in the bump-tool directory

### Setting up GitLab token and app-interface projects

1. **Create a personal access token** at your GitLab instance
   - Navigate to User Settings → Access Tokens
   - Select scopes: `api` and `write_repository`
   - Generate and copy the token

2. **Add these values to `.env`**
   - `GITLAB_TOKEN`
   - `APP_INTERFACE_FORK_PROJECT` (your fork path)
   - `APP_INTERFACE_UPSTREAM_PROJECT` (usually `service/app-interface`)
   - Optional: `GITLAB_BASE_URL`, `APP_INTERFACE_TARGET_BRANCH`

## Controls

| Key | Action |
|-----|--------|
| **↑/↓** | Navigate projects |
| **d** | View commit details |
| **enter** | Bump selected project |
| **q** | Quit |

## After Bumping

The tool now performs the full workflow for you:

1. Sync fork branch from upstream `service/app-interface`
2. Update refs in the deployment file
3. Commit and push a generated bump branch
4. Open a GitLab merge request targeting upstream
5. Create a JIRA ticket with the changelog and MR link

Example output:
```
📋 MERGE REQUEST CREATED
--------------------------------------------------------------------------------
Project: service/app-interface
Branch:  bump/landing-page-frontend-def5678-1713888800
MR URL:  https://gitlab.com/service/app-interface/-/merge_requests/1234
--------------------------------------------------------------------------------

🎫 JIRA TICKET CREATED
--------------------------------------------------------------------------------
Issue Key: RHCLOUD-12345
JIRA URL:  https://redhat.atlassian.net/browse/RHCLOUD-12345
--------------------------------------------------------------------------------
```

## Adding New Projects

Edit `bump-brat-config.yml`:

`bump-brat` now finds and updates `ref:` values by matching `name: <project>` blocks inside the deployment YAML, so no line numbers are needed.

```yaml
projects:
  - name: your-frontend
    file_path: data/services/insights/your-service/deploy.yml
    repo_url: https://github.com/RedHatInsights/your-frontend
```

## JIRA Configuration

JIRA tickets are automatically created when you bump a project. Configure the JIRA fields in `jira/jira_fields_config.json`:

```bash
cp jira/jira_fields_config.json.example jira/jira_fields_config.json
# Edit jira/jira_fields_config.json with your team's settings
```

Example configuration:
```json
{
  "components": ["Console UI"],
  "labels": ["platform-experience-ui"],
  "team_field": "customfield_10001",
  "team": "cc1c0d99-0567-45c8-bf77-8e6149d7ed83"
}
```

The tool automatically creates tickets with:
- **Summary**: `Bump {project-name} to {short-hash}`
- **Description**: Changelog with commit links + MR URL
- **Components, Labels, Team**: From `jira_fields_config.json`

See `jira/README.md` for more details on configuration.

## Building from Source

```bash
make build
./bump-brat
```

## Make Targets

```bash
make build  # build bump-brat binary
make run    # run directly with go run
make clean  # remove built binary
```
