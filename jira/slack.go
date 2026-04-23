package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func postToSlack(webhookURL, ticketKey, ticketURL, summary, description string, config *Config) error {
	payload := map[string]string{
		"jira_ticket_key": ticketKey,
		"jira_ticket_url": ticketURL,
		"summary":         summary,
		"description":     description,
		"team":            config.Team,
		"labels":          joinStrings(config.Labels),
		"components":      joinStrings(config.Components),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Slack request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Slack webhook returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
