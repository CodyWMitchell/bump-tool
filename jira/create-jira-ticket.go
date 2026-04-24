package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config represents the JIRA fields configuration
type Config struct {
	Components []string `json:"components"`
	Labels     []string `json:"labels"`
	TeamField  string   `json:"team_field"`
	Team       string   `json:"team"`
}

// JiraPayload represents the JIRA issue creation payload
type JiraPayload struct {
	Fields map[string]interface{} `json:"fields"`
}

// JiraResponse represents the response from JIRA
type JiraResponse struct {
	Key string `json:"key"`
}

func main() {
	// Command-line flags
	summary := flag.String("summary", "", "Ticket summary/title (required)")
	description := flag.String("description", "", "Ticket description (required)")
	dryRun := flag.Bool("dry-run", false, "Print payload without creating ticket")
	configPath := flag.String("config", "jira_fields_config.json", "Path to config file")
	postToSlackFlag := flag.Bool("post-to-slack", false, "Post ticket to Slack workflow")
	flag.Parse()

	if *summary == "" || *description == "" {
		fmt.Println("Error: --summary and --description are required")
		flag.Usage()
		os.Exit(1)
	}

	// Load .env from current directory and parent directory
	godotenv.Load()
	godotenv.Load(filepath.Join("..", ".env"))

	// Get JIRA credentials from environment
	jiraURL := os.Getenv("JIRA_URL")
	jiraUsername := os.Getenv("JIRA_USERNAME")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")

	if jiraURL == "" || jiraUsername == "" || jiraAPIToken == "" {
		fmt.Println("Error: Missing JIRA credentials in environment variables")
		fmt.Println("Required: JIRA_URL, JIRA_USERNAME, JIRA_API_TOKEN")
		os.Exit(1)
	}

	// Load config
	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Build JIRA payload
	payload := buildPayload(*summary, *description, config)

	if *dryRun {
		printDryRun(payload, config, jiraURL, jiraUsername)
		return
	}

	// Create the ticket
	issueKey, err := createTicket(jiraURL, jiraUsername, jiraAPIToken, payload)
	if err != nil {
		fmt.Printf("\n✗ Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	// Print success
	fmt.Printf("\n✓ Successfully created ticket: %s\n", issueKey)
	ticketURL := fmt.Sprintf("%s/browse/%s", jiraURL, issueKey)
	fmt.Printf("  URL: %s\n", ticketURL)
	fmt.Printf("  Team: %s\n", config.Team)
	fmt.Printf("  Labels: %s\n", joinStrings(config.Labels))
	fmt.Printf("  Components: %s\n", joinStrings(config.Components))

	// Post to Slack if flag is set
	if *postToSlackFlag {
		slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
		if slackWebhookURL == "" {
			fmt.Printf("\n⚠ Warning: SLACK_WEBHOOK_URL not set in environment variables\n")
		} else {
			if err := postToSlack(slackWebhookURL, issueKey, ticketURL, *summary, *description, config); err != nil {
				fmt.Printf("\n⚠ Warning: Failed to post to Slack: %v\n", err)
			} else {
				fmt.Printf("\n✓ Posted to Slack\n")
			}
		}
	}
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s", path)
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	return &config, nil
}

func buildPayload(summary, description string, config *Config) *JiraPayload {
	fields := map[string]interface{}{
		"project":     map[string]string{"key": "RHCLOUD"},
		"summary":     summary,
		"description": description,
		"issuetype":   map[string]string{"name": "Story"},
		"labels":      config.Labels,
	}

	// Add components if any
	if len(config.Components) > 0 {
		components := make([]map[string]string, len(config.Components))
		for i, comp := range config.Components {
			components[i] = map[string]string{"name": comp}
		}
		fields["components"] = components
	}

	// Add team if field ID is configured
	if config.Team != "" && config.TeamField != "" {
		fields[config.TeamField] = config.Team
	}

	return &JiraPayload{Fields: fields}
}

func printDryRun(payload *JiraPayload, config *Config, jiraURL, username string) {
	fmt.Println("\n" + strings("=", 80))
	fmt.Println("DRY RUN - Would create ticket with the following payload:")
	fmt.Println(strings("=", 80))

	jsonBytes, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(jsonBytes))

	fmt.Println(strings("=", 80))
	fmt.Printf("\nTeam: %s\n", config.Team)
	fmt.Printf("Labels: %s\n", joinStrings(config.Labels))
	fmt.Printf("Components: %s\n", joinStrings(config.Components))
	fmt.Printf("JIRA URL: %s\n", jiraURL)
	fmt.Printf("Username: %s\n", username)
}

func createTicket(jiraURL, username, apiToken string, payload *JiraPayload) (string, error) {
	url := fmt.Sprintf("%s/rest/api/2/issue", jiraURL)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.SetBasicAuth(username, apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("%d %s\n  Details: %s", resp.StatusCode, resp.Status, string(body))
	}

	var result JiraResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	return result.Key, nil
}

func joinStrings(arr []string) string {
	if len(arr) == 0 {
		return ""
	}
	result := arr[0]
	for i := 1; i < len(arr); i++ {
		result += ", " + arr[i]
	}
	return result
}

func strings(char string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += char
	}
	return result
}
