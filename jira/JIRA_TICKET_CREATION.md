# JIRA Ticket Creation

Create JIRA tickets using static configuration from `jira_fields_config.json`.

## Files

- **`create-jira-ticket`** - Go binary to create JIRA tickets
- **`jira_fields_config.json`** - Configuration for ticket fields
- **`.env`** - JIRA credentials (in repo root)

## Setup

### 1. Configure Credentials

Make sure your `.env` file (in repo root) has JIRA credentials:

```bash
JIRA_URL=https://redhat.atlassian.net
JIRA_USERNAME=your-email@redhat.com
JIRA_API_TOKEN=your-jira-api-token
```

Generate your API token at: https://id.atlassian.com/manage-profile/security/api-tokens

### 2. Build the Tool

```bash
cd jira
make
```

### 3. Get Your Team ID (Optional)

If you want to populate the Team field in JIRA, get your team ID from your Atlassian team URL:

```
https://home.atlassian.com/.../people/team/cc1c0d99-0567-45c8-bf77-8e6149d7ed83?...
                                          ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                                          This is your team ID
```

### 4. Configure Ticket Fields

Edit `jira_fields_config.json` with your desired values:

```json
{
  "components": [
    "AI Framework"
  ],
  "labels": [
    "hcc-ai-framework"
  ],
  "team_field": "customfield_10001",
  "team": "your-team-uuid-here"
}
```

**Notes**:
- `team` should be your team's UUID from your Atlassian team URL
- Set `team_field` to empty string `""` if you don't want to set the team field

## Usage

If you generated a bump MR with bump-brat and enabled `BUMP_BRAT_USE_CLAUDE_SUMMARY=true`, reuse the printed AI summary blurb in your JIRA `--description` for a consistent short changelog.

### Basic Usage

```bash
./create-jira-ticket \
    --summary "Fix login bug" \
    --description "Users can't log in after upgrade"
```

### Dry Run (Preview Without Creating)

```bash
./create-jira-ticket \
    --summary "Test ticket" \
    --description "Testing the script" \
    --dry-run
```

### Command-Line Flags

```
--summary string       Ticket summary/title (required)
--description string   Ticket description (required)
--dry-run             Print payload without creating ticket
--config string       Path to config file (default "jira_fields_config.json")
```

## Configuration

All ticket fields except summary and description are defined in `jira_fields_config.json`:

| Field | Description | Example |
|-------|-------------|---------|
| **components** | JIRA components (array) | `["AI Framework"]` |
| **labels** | JIRA labels (array) | `["hcc-ai-framework", "urgent"]` |
| **team_field** | Custom field ID for team | `"customfield_10001"` or `""` |
| **team** | Team UUID from Atlassian URL | `"cc1c0d99-0567-45c8-bf77-8e6149d7ed83"` |

The tool always creates tickets in project `RHCLOUD` with issue type `Story`.

## Fields Populated

The tool automatically sets these fields:

| Field | Source | Notes |
|-------|--------|-------|
| **Project** | Hardcoded | RHCLOUD |
| **Issue Type** | Hardcoded | Story |
| **Summary** | Command line arg | Required |
| **Description** | Command line arg | Required |
| **Components** | Config file | From `jira_fields_config.json` |
| **Labels** | Config file | From `jira_fields_config.json` |
| **Team** | Config file | From `jira_fields_config.json` (if `team_field` is set) |
| **Reporter** | Automatic | JIRA sets to authenticated user |

## Examples

### Example 1: Simple Bug Ticket

```bash
./create-jira-ticket \
    --summary "Login form validation broken" \
    --description "Email validation not working in login form"
```

### Example 2: Feature Request

```bash
./create-jira-ticket \
    --summary "Implement new analytics dashboard" \
    --description "Create comprehensive analytics dashboard with real-time metrics"
```

## Troubleshooting

### "Missing JIRA credentials"

Make sure the `.env` file in repo root contains:
- `JIRA_URL`
- `JIRA_USERNAME`
- `JIRA_API_TOKEN`

### "Config file not found"

Make sure `jira_fields_config.json` exists in the current directory.

### Authentication Errors

1. Verify your API token is valid
2. Make sure your username (email) is correct
3. Check that you have permission to create issues in the RHCLOUD project

### Team Field Errors

If you get errors about the team field:
1. Make sure you're using the team UUID (from your Atlassian team URL), not the team name
2. Verify the `team_field` ID is correct (usually `customfield_10001`)
3. If you don't need the team field, set `team_field` to empty string `""` in the config
