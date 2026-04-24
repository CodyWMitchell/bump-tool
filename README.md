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

# Claude Code summary blurb (optional; off by default)
BUMP_BRAT_USE_CLAUDE_SUMMARY=false
# Optional: set to force `claude --resume <session-id>`
BUMP_BRAT_CLAUDE_SESSION_ID=
```

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

### Optional: AI commit summary via local Claude Code session

If you want MR descriptions to include a short AI blurb that summarizes the full commit set:

1. Enable:
   - `BUMP_BRAT_USE_CLAUDE_SUMMARY=true`
2. Optional: if you specifically want to reuse a known Claude session, also set:
   - `BUMP_BRAT_CLAUDE_SESSION_ID=<session-id>`

When enabled, bump-brat uses your local Claude auth/session context by default to generate a concise summary from all included commits. If `BUMP_BRAT_CLAUDE_SESSION_ID` is set, it will use `--resume` for that session. The same blurb is printed in terminal output so you can reuse it in JIRA descriptions.

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

Example output:
```
📋 MERGE REQUEST CREATED
--------------------------------------------------------------------------------
Project: service/app-interface
Branch:  bump/landing-page-frontend-def5678-1713888800
MR URL:  https://gitlab.com/service/app-interface/-/merge_requests/1234
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

## Creating JIRA Tickets

JIRA ticket creation tools are in the `jira/` directory:

```bash
cd jira

# Build the tool
make

# Configure your fields
cp jira_fields_config.json.example jira_fields_config.json
# Edit jira_fields_config.json

# Create a ticket
./create-jira-ticket \
    --summary "Bump frontend hashes" \
    --description "Update production hashes for Q2 release"
```

See `jira/README.md` for quick start and `jira/JIRA_TICKET_CREATION.md` for full documentation.

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
