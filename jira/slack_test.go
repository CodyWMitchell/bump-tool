package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostToSlack(t *testing.T) {
	tests := []struct {
		name           string
		ticketKey      string
		ticketURL      string
		summary        string
		description    string
		config         *Config
		serverResponse int
		expectError    bool
	}{
		{
			name:        "Successful post",
			ticketKey:   "RHCLOUD-12345",
			ticketURL:   "https://redhat.atlassian.net/browse/RHCLOUD-12345",
			summary:     "Test ticket summary",
			description: "Test ticket description",
			config: &Config{
				Team:       "platform-experience-ui",
				Labels:     []string{"automation", "frontend"},
				Components: []string{"Console UI"},
			},
			serverResponse: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Server error",
			ticketKey:   "RHCLOUD-12345",
			ticketURL:   "https://redhat.atlassian.net/browse/RHCLOUD-12345",
			summary:     "Test ticket",
			description: "Description",
			config: &Config{
				Team:       "test-team",
				Labels:     []string{"test"},
				Components: []string{"Test Component"},
			},
			serverResponse: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:        "Bad request",
			ticketKey:   "RHCLOUD-12345",
			ticketURL:   "https://redhat.atlassian.net/browse/RHCLOUD-12345",
			summary:     "Test",
			description: "Test",
			config: &Config{
				Team:       "team",
				Labels:     []string{},
				Components: []string{},
			},
			serverResponse: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			var receivedPayload map[string]string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				// Verify content type
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", ct)
				}

				// Read and parse body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Failed to read request body: %v", err)
				}

				if err := json.Unmarshal(body, &receivedPayload); err != nil {
					t.Fatalf("Failed to parse request body: %v", err)
				}

				// Send response
				w.WriteHeader(tt.serverResponse)
				if tt.serverResponse != http.StatusOK {
					w.Write([]byte(`{"error": "Something went wrong"}`))
				}
			}))
			defer server.Close()

			// Call the function
			err := postToSlack(server.URL, tt.ticketKey, tt.ticketURL, tt.summary, tt.description, tt.config)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			// Verify payload if successful
			if !tt.expectError {
				if receivedPayload["jira_ticket_key"] != tt.ticketKey {
					t.Errorf("Expected jira_ticket_key %q, got %q", tt.ticketKey, receivedPayload["jira_ticket_key"])
				}
				if receivedPayload["jira_ticket_url"] != tt.ticketURL {
					t.Errorf("Expected jira_ticket_url %q, got %q", tt.ticketURL, receivedPayload["jira_ticket_url"])
				}
				if receivedPayload["summary"] != tt.summary {
					t.Errorf("Expected summary %q, got %q", tt.summary, receivedPayload["summary"])
				}
				if receivedPayload["description"] != tt.description {
					t.Errorf("Expected description %q, got %q", tt.description, receivedPayload["description"])
				}
				if receivedPayload["team"] != tt.config.Team {
					t.Errorf("Expected team %q, got %q", tt.config.Team, receivedPayload["team"])
				}

				expectedLabels := joinStrings(tt.config.Labels)
				if receivedPayload["labels"] != expectedLabels {
					t.Errorf("Expected labels %q, got %q", expectedLabels, receivedPayload["labels"])
				}

				expectedComponents := joinStrings(tt.config.Components)
				if receivedPayload["components"] != expectedComponents {
					t.Errorf("Expected components %q, got %q", expectedComponents, receivedPayload["components"])
				}
			}
		})
	}
}

func TestPostToSlackPayloadStructure(t *testing.T) {
	// Test that the payload structure matches expected format
	config := &Config{
		Team:       "platform-experience-ui",
		Labels:     []string{"automation", "frontend", "urgent"},
		Components: []string{"Console UI", "API"},
	}

	var receivedPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ticketKey := "RHCLOUD-99999"
	ticketURL := "https://redhat.atlassian.net/browse/RHCLOUD-99999"
	summary := "Critical bug in production"
	description := "Detailed description of the issue"

	err := postToSlack(server.URL, ticketKey, ticketURL, summary, description, config)
	if err != nil {
		t.Fatalf("postToSlack failed: %v", err)
	}

	// Verify all required fields are present
	requiredFields := []string{
		"jira_ticket_key",
		"jira_ticket_url",
		"summary",
		"description",
		"team",
		"labels",
		"components",
	}

	for _, field := range requiredFields {
		if _, exists := receivedPayload[field]; !exists {
			t.Errorf("Expected field %q in payload, but it's missing", field)
		}
	}

	// Verify field values
	if receivedPayload["jira_ticket_key"] != ticketKey {
		t.Errorf("Incorrect ticket key in payload")
	}
	if receivedPayload["jira_ticket_url"] != ticketURL {
		t.Errorf("Incorrect ticket URL in payload")
	}
	if receivedPayload["summary"] != summary {
		t.Errorf("Incorrect summary in payload")
	}
	if receivedPayload["description"] != description {
		t.Errorf("Incorrect description in payload")
	}
	if receivedPayload["team"] != config.Team {
		t.Errorf("Incorrect team in payload")
	}
	if receivedPayload["labels"] != "automation, frontend, urgent" {
		t.Errorf("Expected labels 'automation, frontend, urgent', got %q", receivedPayload["labels"])
	}
	if receivedPayload["components"] != "Console UI, API" {
		t.Errorf("Expected components 'Console UI, API', got %q", receivedPayload["components"])
	}
}

func TestPostToSlackEmptyFields(t *testing.T) {
	// Test with empty labels and components
	config := &Config{
		Team:       "test-team",
		Labels:     []string{},
		Components: []string{},
	}

	var receivedPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := postToSlack(server.URL, "KEY-123", "https://example.com", "Summary", "Description", config)
	if err != nil {
		t.Fatalf("postToSlack failed with empty fields: %v", err)
	}

	// Empty arrays should result in empty strings
	if receivedPayload["labels"] != "" {
		t.Errorf("Expected empty labels string, got %q", receivedPayload["labels"])
	}
	if receivedPayload["components"] != "" {
		t.Errorf("Expected empty components string, got %q", receivedPayload["components"])
	}
}

func TestPostToSlackInvalidURL(t *testing.T) {
	config := &Config{
		Team:       "test-team",
		Labels:     []string{"test"},
		Components: []string{"Test"},
	}

	// Test with invalid URL
	err := postToSlack("not-a-valid-url", "KEY-123", "https://example.com", "Summary", "Description", config)
	if err == nil {
		t.Error("Expected error with invalid URL, got nil")
	}
}

func TestPostToSlackNetworkError(t *testing.T) {
	config := &Config{
		Team:       "test-team",
		Labels:     []string{"test"},
		Components: []string{"Test"},
	}

	// Test with unreachable server
	err := postToSlack("http://localhost:1", "KEY-123", "https://example.com", "Summary", "Description", config)
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}
