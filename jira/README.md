# JIRA Ticket Creation Tools

Create JIRA tickets with predefined fields from configuration.

## Quick Start

```bash
cd jira-tools

# 1. Install dependencies (from repo root)
pip3 install -r ../requirements.txt

# 2. Configure your fields
cp jira_fields_config.json.example jira_fields_config.json
# Edit jira_fields_config.json

# 3. Create a ticket
python3 create_jira_ticket.py \
    --summary "Your ticket title" \
    --description "Your ticket description" \
    --dry-run
```

## Files

- **`create_jira_ticket.py`** - Main script to create JIRA tickets
- **`jira_fields_config.json`** - Configuration for ticket fields
- **`find_team_field.py`** - Helper to find team field ID in your JIRA instance
- **`test_jira_create.sh`** - Test script with examples
- **`JIRA_TICKET_CREATION.md`** - Full documentation

## Configuration

Edit `jira_fields_config.json`:

```json
{
  "components": [
    "Console UI"
  ],
  "labels": [
    "platform-experience-ui"
  ],
  "team_field": "customfield_10001",
  "team": "cc1c0d99-0567-45c8-bf77-8e6149d7ed83"
}
```

**Note**: The `team` value should be the team ID (UUID), not the team name. Get it from your team's Atlassian URL:
```
https://home.atlassian.com/.../people/team/YOUR-TEAM-ID-HERE?cloudId=...
                                          ^^^^^^^^^^^^^^^^^^^^
```

JIRA credentials are read from `../.env` (the repo root):

```bash
JIRA_URL=https://redhat.atlassian.net
JIRA_USERNAME=your-email@redhat.com
JIRA_API_TOKEN=your-jira-api-token
```

## Usage

### Create a Ticket

```bash
python3 create_jira_ticket.py \
    --summary "Fix login bug" \
    --description "Users can't log in after upgrade"
```

### Dry Run (Preview)

```bash
python3 create_jira_ticket.py \
    --summary "Test ticket" \
    --description "Testing" \
    --dry-run
```

### Get Your Team ID

Get your team ID from your team's Atlassian URL:

1. Go to your team page in Atlassian
2. Copy the UUID from the URL:
   ```
   https://home.atlassian.com/.../people/team/YOUR-TEAM-ID?...
   ```
3. Use that UUID as the `team` value in your config

See **JIRA_TICKET_CREATION.md** for complete documentation.
