#!/usr/bin/env python3
"""
Create JIRA tickets using static configuration from jira_fields_config.json

Usage:
    python create_jira_ticket.py --summary "Fix login bug" --description "Users can't log in"
    python create_jira_ticket.py --summary "Add new feature" --description "..." --dry-run
"""

import argparse
import json
import os
import sys
from typing import Dict, Any, Optional
import requests
from requests.auth import HTTPBasicAuth
from dotenv import load_dotenv

# Load environment variables from .env file (look in parent directory too)
load_dotenv()
load_dotenv(os.path.join(os.path.dirname(__file__), '..', '.env'))


def load_config(config_path: str = "jira_fields_config.json") -> Dict[str, Any]:
    """Load field configuration from JSON file."""
    if not os.path.exists(config_path):
        print(f"Error: Config file not found at {config_path}")
        sys.exit(1)

    with open(config_path, 'r') as f:
        return json.load(f)


def get_jira_credentials() -> tuple[str, str, str]:
    """Get JIRA credentials from environment variables."""
    jira_url = os.getenv('JIRA_URL')
    jira_username = os.getenv('JIRA_USERNAME')
    jira_api_token = os.getenv('JIRA_API_TOKEN')

    if not all([jira_url, jira_username, jira_api_token]):
        print("Error: Missing JIRA credentials in environment variables")
        print("Required: JIRA_URL, JIRA_USERNAME, JIRA_API_TOKEN")
        sys.exit(1)

    return jira_url, jira_username, jira_api_token


def create_jira_ticket(
    summary: str,
    description: str,
    dry_run: bool = False,
    config_path: str = "jira_fields_config.json"
) -> Optional[Dict[str, Any]]:
    """
    Create a JIRA ticket using fields from config file.

    Args:
        summary: Ticket summary/title
        description: Ticket description
        dry_run: If True, print payload without creating ticket
        config_path: Path to config file

    Returns:
        Created issue data or None if dry run or error
    """
    # Load configuration
    config = load_config(config_path)

    # Get fields from config
    components = config.get('components', [])
    labels = config.get('labels', [])
    team = config.get('team', '')
    team_field = config.get('team_field')  # Custom field ID for team

    # Get JIRA credentials
    jira_url, jira_username, jira_api_token = get_jira_credentials()
    auth = HTTPBasicAuth(jira_username, jira_api_token)

    # Build JIRA payload
    payload = {
        "fields": {
            "project": {"key": "RHCLOUD"},
            "summary": summary,
            "description": description,
            "issuetype": {"name": "Story"},
            "labels": labels,
        }
    }

    # Add components if any
    if components:
        payload["fields"]["components"] = [{"name": comp} for comp in components]

    # Add team if field ID is configured
    # Try different formats - some JIRA instances want just the string, others want an ID
    if team and team_field:
        # If team looks like an ID (starts with digits), use it as-is
        # Otherwise, try as a string value (for text fields)
        if team.isdigit() or team.startswith('team-'):
            payload["fields"][team_field] = team
        else:
            # For text/string fields, just use the string directly
            payload["fields"][team_field] = team

    if dry_run:
        print("\n" + "="*80)
        print("DRY RUN - Would create ticket with the following payload:")
        print("="*80)
        print(json.dumps(payload, indent=2))
        print("="*80)
        print(f"\nTeam: {team}")
        print(f"Labels: {', '.join(labels)}")
        print(f"Components: {', '.join(components)}")
        print(f"JIRA URL: {jira_url}")
        print(f"Username: {jira_username}")
        return None

    # Create the ticket
    try:
        response = requests.post(
            f"{jira_url}/rest/api/2/issue",
            auth=auth,
            headers={'Content-Type': 'application/json'},
            json=payload
        )
        response.raise_for_status()

        result = response.json()
        issue_key = result.get('key')

        print(f"\n✓ Successfully created ticket: {issue_key}")
        print(f"  URL: {jira_url}/browse/{issue_key}")
        print(f"  Team: {team}")
        print(f"  Labels: {', '.join(labels)}")
        print(f"  Components: {', '.join(components)}")

        return result

    except requests.exceptions.RequestException as e:
        print(f"\n✗ Error creating ticket: {e}")
        if hasattr(e, 'response') and e.response is not None:
            try:
                error_detail = e.response.json()
                print(f"  Details: {json.dumps(error_detail, indent=2)}")
            except:
                print(f"  Response: {e.response.text}")
        return None


def main():
    parser = argparse.ArgumentParser(
        description='Create JIRA tickets using configuration from jira_fields_config.json',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Create a ticket with dry run
  python create_jira_ticket.py \\
      --summary "Fix login bug" \\
      --description "Users cannot log in after upgrade" \\
      --dry-run

  # Create a real ticket
  python create_jira_ticket.py \\
      --summary "Add new dashboard" \\
      --description "Create analytics dashboard with real-time metrics"

Note: Components, labels, and team are read from jira_fields_config.json
        """
    )

    parser.add_argument('--summary', required=True, help='Ticket summary/title')
    parser.add_argument('--description', required=True, help='Ticket description')
    parser.add_argument('--dry-run', action='store_true', help='Print payload without creating ticket')
    parser.add_argument('--config', default='jira_fields_config.json', help='Path to config file')

    args = parser.parse_args()

    create_jira_ticket(
        summary=args.summary,
        description=args.description,
        dry_run=args.dry_run,
        config_path=args.config
    )


if __name__ == '__main__':
    main()
