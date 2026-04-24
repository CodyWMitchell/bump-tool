package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func postToSlack(webhookURL, ticketKey, ticketURL, summary, description string, config *Config) error {
	// Format single ticket in same format as bulk (project: KEY - URL)
	// Extract project name from summary (format: "Bump project-name to hash")
	projectName := "unknown"
	if len(summary) > 5 && summary[:5] == "Bump " {
		parts := strings.Split(summary[5:], " to ")
		if len(parts) > 0 {
			projectName = parts[0]
		}
	}

	ticketsText := fmt.Sprintf("%s: %s - %s", projectName, ticketKey, ticketURL)

	payload := map[string]string{
		"tickets":    ticketsText,
		"count":      "1",
		"team":       config.Team,
		"labels":     joinStrings(config.Labels),
		"components": joinStrings(config.Components),
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
