# JIRA Ticket Creation

This directory contains a script and configuration for creating JIRA tickets with predefined fields.

## Files

- **`create_jira_ticket.py`** - Script to create JIRA tickets
- **`jira_fields_config.json`** - Static configuration for ticket fields
- **`test_jira_create.sh`** - Test script with example
- **`.env`** - JIRA credentials (copy from `.env.example`)

## Setup

### 1. Configure Credentials

Make sure your `.env` file has JIRA credentials:

```bash
JIRA_URL=https://redhat.atlassian.net
JIRA_USERNAME=your-email@redhat.com
JIRA_API_TOKEN=your-jira-api-token
```

Generate your API token at: https://id.atlassian.com/manage-profile/security/api-tokens

### 2. Install Dependencies

```bash
pip3 install -r requirements.txt
```

The script automatically loads credentials from `.env` - no need to export or source it manually.

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
- Set `team_field` to `null` if you don't want to set the team field in JIRA

## Usage

### Basic Usage

```bash
python3 create_jira_ticket.py \
    --summary "Fix login bug" \
    --description "Users can't log in after upgrade"
```

### Dry Run (Preview Without Creating)

```bash
python3 create_jira_ticket.py \
    --summary "Test ticket" \
    --description "Testing the script" \
    --dry-run
```

## Configuration

All ticket fields except summary and description are defined in `jira_fields_config.json`:

| Field | Description | Example |
|-------|-------------|---------|
| **components** | JIRA components (array) | `["AI Framework"]` |
| **labels** | JIRA labels (array) | `["hcc-ai-framework", "urgent"]` |
| **team_field** | Custom field ID for team | `"customfield_10001"` or `null` |
| **team** | Team UUID from Atlassian URL | `"cc1c0d99-0567-45c8-bf77-8e6149d7ed83"` |

The script always creates tickets in project `RHCLOUD` with issue type `Story`.

### Changing Fields

To change the default fields for your tickets, edit `jira_fields_config.json`:

```json
{
  "components": [
    "Platform UI",
    "Chrome"
  ],
  "labels": [
    "platform-experience-ui",
    "high-priority"
  ],
  "team": "platform-experience-ui"
}
```

## Fields Populated

The script automatically sets these fields:

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

## Testing

Run the test script to see a dry run example:

```bash
./test_jira_create.sh
```

This will show you what would be created without actually creating a ticket.

## Examples

### Example 1: Simple Bug Ticket

Edit `jira_fields_config.json`:
```json
{
  "components": ["AI Framework"],
  "labels": ["hcc-ai-framework", "bug"],
  "team_field": "customfield_10001",
  "team": "hcc-ai-framework"
}
```

Create the ticket:
```bash
python3 create_jira_ticket.py \
    --summary "Login form validation broken" \
    --description "Email validation not working in login form"
```

### Example 2: Feature Request

Edit `jira_fields_config.json`:
```json
{
  "components": ["Platform UI", "Chrome"],
  "labels": ["platform-experience-ui"],
  "team_field": "customfield_10001",
  "team": "platform-experience-ui"
}
```

Create the ticket:
```bash
python3 create_jira_ticket.py \
    --summary "Implement new analytics dashboard" \
    --description "Create comprehensive analytics dashboard with real-time metrics and drill-down capabilities"
```

### Example 3: Urgent Security Fix

Edit `jira_fields_config.json`:
```json
{
  "components": ["Access Management", "RBAC"],
  "labels": ["hcc-ai-platform-accessmanagement", "security", "urgent"],
  "team_field": "customfield_10001",
  "team": "hcc-ai-platform-accessmanagement"
}
```

Create the ticket:
```bash
python3 create_jira_ticket.py \
    --summary "Patch authentication vulnerability" \
    --description "Apply security patch for CVE-2024-XXXXX in authentication middleware"
```

## Troubleshooting

### "Missing JIRA credentials"

Make sure your `.env` file exists and contains:
- `JIRA_URL`
- `JIRA_USERNAME`
- `JIRA_API_TOKEN`

### "Config file not found"

Make sure `jira_fields_config.json` exists in the same directory as the script.

### Authentication Errors

1. Verify your API token is valid
2. Make sure your username (email) is correct
3. Check that you have permission to create issues in the RHCLOUD project


## Integration with Automation

You can integrate this with CI/CD or automation tools:

```bash
# Edit config before creating ticket
cat > jira_fields_config.json <<EOF
{
  "components": ["AI Framework"],
  "labels": ["hcc-ai-framework", "automation"],
  "team_field": "customfield_10001",
  "team": "hcc-ai-framework"
}
EOF

# Create ticket
python3 create_jira_ticket.py \
    --summary "$SUMMARY" \
    --description "$DESCRIPTION"
```

Or use it in Python code:

```python
from create_jira_ticket import create_jira_ticket

result = create_jira_ticket(
    summary="Automated ticket",
    description="Created by automation"
)

if result:
    print(f"Created: {result['key']}")
```
