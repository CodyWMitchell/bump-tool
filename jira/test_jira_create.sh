#!/bin/bash
# Test script for creating JIRA tickets
# Fields are read from jira_fields_config.json

set -e

echo "========================================="
echo "JIRA Ticket Creation Test Script"
echo "========================================="
echo ""

# Check if .env file exists in parent directory
if [ ! -f ../.env ]; then
    echo "Error: .env file not found in parent directory"
    echo "Please copy .env.example to .env in the repo root and fill in your JIRA credentials"
    exit 1
fi

# Load environment variables
source ../.env

# Verify required variables are set
if [ -z "$JIRA_URL" ] || [ -z "$JIRA_USERNAME" ] || [ -z "$JIRA_API_TOKEN" ]; then
    echo "Error: Missing JIRA credentials in .env file"
    echo "Required: JIRA_URL, JIRA_USERNAME, JIRA_API_TOKEN"
    exit 1
fi

echo "Using JIRA URL: $JIRA_URL"
echo "Using JIRA Username: $JIRA_USERNAME"
echo ""

echo "Current config from jira_fields_config.json:"
echo "--------------------------------------------"
cat jira_fields_config.json
echo ""
echo ""

# Test: Dry run
echo "Test: Dry run ticket creation"
echo "------------------------------"
python3 create_jira_ticket.py \
    --summary "Test ticket: Fix authentication issue" \
    --description "Users are experiencing intermittent authentication failures when logging in during peak hours. This needs investigation and a fix." \
    --dry-run

echo ""
echo ""
echo "========================================="
echo "Test completed (dry run)"
echo "========================================="
echo ""
echo "To create an actual ticket, remove the --dry-run flag:"
echo ""
echo "python3 create_jira_ticket.py \\"
echo "    --summary \"Your ticket summary\" \\"
echo "    --description \"Your ticket description\""
echo ""
echo "To change story points, labels, components, or team,"
echo "edit jira_fields_config.json"
echo ""
