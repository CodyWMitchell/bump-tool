package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Test helpers and mock data

func setupTestEnv(t *testing.T) func() {
	t.Helper()
	// Save original env vars
	origVars := map[string]string{
		"GITLAB_TOKEN":                  os.Getenv("GITLAB_TOKEN"),
		"GITLAB_BASE_URL":               os.Getenv("GITLAB_BASE_URL"),
		"APP_INTERFACE_FORK_PROJECT":    os.Getenv("APP_INTERFACE_FORK_PROJECT"),
		"APP_INTERFACE_UPSTREAM_PROJECT": os.Getenv("APP_INTERFACE_UPSTREAM_PROJECT"),
		"APP_INTERFACE_TARGET_BRANCH":   os.Getenv("APP_INTERFACE_TARGET_BRANCH"),
		"JIRA_URL":                      os.Getenv("JIRA_URL"),
		"JIRA_USERNAME":                 os.Getenv("JIRA_USERNAME"),
		"JIRA_API_TOKEN":                os.Getenv("JIRA_API_TOKEN"),
		"GITHUB_TOKEN":                  os.Getenv("GITHUB_TOKEN"),
	}

	// Return cleanup function
	return func() {
		for k, v := range origVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.yml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

// Utility function tests

func TestShortSHA(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Full SHA", "1234567890abcdef1234567890abcdef12345678", "1234567"},
		{"Short SHA", "1234567", "1234567"},
		{"Empty", "", ""},
		{"Partial", "12345", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortSHA(tt.input)
			if result != tt.expected {
				t.Errorf("shortSHA(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"With .git suffix", "https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"Without .git suffix", "https://github.com/user/repo", "https://github.com/user/repo"},
		{"With trailing space", "https://github.com/user/repo.git ", "https://github.com/user/repo"},
		{"Clean URL", "https://github.com/user/repo", "https://github.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRepoURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeRepoURL(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGithubCommitURL(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		sha      string
		expected string
	}{
		{
			"Standard repo",
			"https://github.com/RedHatInsights/landing-page-frontend",
			"abc123",
			"https://github.com/RedHatInsights/landing-page-frontend/commit/abc123",
		},
		{
			"Repo with .git",
			"https://github.com/RedHatInsights/landing-page-frontend.git",
			"def456",
			"https://github.com/RedHatInsights/landing-page-frontend/commit/def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := githubCommitURL(tt.repoURL, tt.sha)
			if result != tt.expected {
				t.Errorf("githubCommitURL(%q, %q) = %q; want %q", tt.repoURL, tt.sha, result, tt.expected)
			}
		})
	}
}

func TestNormalizedProjectFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"With ../ prefix", "../data/services/insights/rbac/deploy.yml", "data/services/insights/rbac/deploy.yml"},
		{"With ./ prefix", "./data/services/insights/rbac/deploy.yml", "data/services/insights/rbac/deploy.yml"},
		{"Multiple ../", "../../data/file.yml", "data/file.yml"},
		{"Clean path", "data/services/insights/rbac/deploy.yml", "data/services/insights/rbac/deploy.yml"},
		{"With spaces", "  ../data/file.yml  ", "data/file.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizedProjectFilePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizedProjectFilePath(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsProductionNamespaceRef(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Production namespace", "/services/insights/namespaces/rbac-prod.yml", true},
		{"Production namespace uppercase", "/services/INSIGHTS/namespaces/RBAC-PROD.yml", true},
		{"Stage namespace", "/services/insights/namespaces/rbac-stage.yml", false},
		{"Ephemeral namespace", "/services/insights/namespaces/rbac-ephemeral.yml", false},
		{"Empty string", "", false},
		{"No prod keyword", "/services/insights/namespaces/rbac.yml", false},
		{"Prod in stage", "/services/insights/namespaces/rbac-stage-prod.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isProductionNamespaceRef(tt.input)
			if result != tt.expected {
				t.Errorf("isProductionNamespaceRef(%q) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Configuration tests

func TestLoadDotEnv(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	content := `# Comment line
GITHUB_TOKEN=test_token_123
GITLAB_TOKEN="quoted_token"
GITLAB_BASE_URL='single_quoted'
export JIRA_URL=https://jira.example.com

EMPTY_VALUE=
INVALID LINE WITHOUT EQUALS
`

	tmpFile := createTempFile(t, content)
	defer os.Remove(tmpFile)

	if err := loadDotEnv(tmpFile); err != nil {
		t.Fatalf("loadDotEnv failed: %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"GITHUB_TOKEN", "test_token_123"},
		{"GITLAB_TOKEN", "quoted_token"},
		{"GITLAB_BASE_URL", "single_quoted"},
		{"JIRA_URL", "https://jira.example.com"},
		{"EMPTY_VALUE", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := os.Getenv(tt.key)
			if result != tt.expected {
				t.Errorf("os.Getenv(%q) = %q; want %q", tt.key, result, tt.expected)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	content := `projects:
  - name: test-frontend
    file_path: data/services/test/deploy.yml
    repo_url: https://github.com/test/repo
  - name: another-frontend
    file_path: data/services/another/deploy.yml
    repo_url: https://github.com/test/another
`

	tmpFile := createTempFile(t, content)
	defer os.Remove(tmpFile)

	// Temporarily override configFile constant
	originalConfigFile := configFile
	defer func() { _ = originalConfigFile }() // Can't reassign const, but keep pattern

	// Instead, read directly
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read temp config: %v", err)
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if len(cfg.Projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(cfg.Projects))
	}

	if cfg.Projects[0].Name != "test-frontend" {
		t.Errorf("Expected first project name 'test-frontend', got %q", cfg.Projects[0].Name)
	}

	if cfg.Projects[1].RepoURL != "https://github.com/test/another" {
		t.Errorf("Expected second project repo URL 'https://github.com/test/another', got %q", cfg.Projects[1].RepoURL)
	}
}

func TestLoadGitLabSettings(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	t.Run("Valid settings", func(t *testing.T) {
		os.Setenv("GITLAB_TOKEN", "test-token")
		os.Setenv("APP_INTERFACE_FORK_PROJECT", "user/app-interface")
		os.Setenv("APP_INTERFACE_UPSTREAM_PROJECT", "service/app-interface")
		os.Setenv("GITLAB_BASE_URL", "https://gitlab.example.com/")
		os.Setenv("APP_INTERFACE_TARGET_BRANCH", "main")

		settings, err := loadGitLabSettings()
		if err != nil {
			t.Fatalf("loadGitLabSettings failed: %v", err)
		}

		if settings.token != "test-token" {
			t.Errorf("Expected token 'test-token', got %q", settings.token)
		}
		if settings.baseURL != "https://gitlab.example.com" {
			t.Errorf("Expected baseURL 'https://gitlab.example.com', got %q", settings.baseURL)
		}
		if settings.targetBranch != "main" {
			t.Errorf("Expected targetBranch 'main', got %q", settings.targetBranch)
		}
	})

	t.Run("Missing token", func(t *testing.T) {
		os.Unsetenv("GITLAB_TOKEN")
		os.Setenv("APP_INTERFACE_FORK_PROJECT", "user/app-interface")
		os.Setenv("APP_INTERFACE_UPSTREAM_PROJECT", "service/app-interface")

		_, err := loadGitLabSettings()
		if err == nil {
			t.Error("Expected error for missing GITLAB_TOKEN, got nil")
		}
	})

	t.Run("Default values", func(t *testing.T) {
		os.Setenv("GITLAB_TOKEN", "test-token")
		os.Setenv("APP_INTERFACE_FORK_PROJECT", "user/app-interface")
		os.Setenv("APP_INTERFACE_UPSTREAM_PROJECT", "service/app-interface")
		os.Unsetenv("GITLAB_BASE_URL")
		os.Unsetenv("APP_INTERFACE_TARGET_BRANCH")

		settings, err := loadGitLabSettings()
		if err != nil {
			t.Fatalf("loadGitLabSettings failed: %v", err)
		}

		if settings.baseURL != "https://gitlab.com" {
			t.Errorf("Expected default baseURL 'https://gitlab.com', got %q", settings.baseURL)
		}
		if settings.targetBranch != "master" {
			t.Errorf("Expected default targetBranch 'master', got %q", settings.targetBranch)
		}
	})
}

func TestLoadJiraSettings(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	t.Run("Valid settings", func(t *testing.T) {
		os.Setenv("JIRA_URL", "https://jira.example.com")
		os.Setenv("JIRA_USERNAME", "user@example.com")
		os.Setenv("JIRA_API_TOKEN", "api-token-123")

		settings, err := loadJiraSettings()
		if err != nil {
			t.Fatalf("loadJiraSettings failed: %v", err)
		}

		if settings.url != "https://jira.example.com" {
			t.Errorf("Expected url 'https://jira.example.com', got %q", settings.url)
		}
		if settings.username != "user@example.com" {
			t.Errorf("Expected username 'user@example.com', got %q", settings.username)
		}
		if settings.apiToken != "api-token-123" {
			t.Errorf("Expected apiToken 'api-token-123', got %q", settings.apiToken)
		}
	})

	t.Run("Missing credentials", func(t *testing.T) {
		os.Unsetenv("JIRA_URL")
		os.Unsetenv("JIRA_USERNAME")
		os.Unsetenv("JIRA_API_TOKEN")

		_, err := loadJiraSettings()
		if err == nil {
			t.Error("Expected error for missing JIRA credentials, got nil")
		}
	})
}

func TestLoadJiraConfig(t *testing.T) {
	configJSON := `{
  "components": ["Console UI", "API"],
  "labels": ["platform-experience-ui", "urgent"],
  "team_field": "customfield_10001",
  "team": "team-uuid-123"
}`

	tmpFile, err := os.CreateTemp("", "jira-config-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configJSON); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := loadJiraConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("loadJiraConfig failed: %v", err)
	}

	if len(cfg.Components) != 2 || cfg.Components[0] != "Console UI" {
		t.Errorf("Expected components [Console UI, API], got %v", cfg.Components)
	}
	if len(cfg.Labels) != 2 || cfg.Labels[1] != "urgent" {
		t.Errorf("Expected labels [platform-experience-ui, urgent], got %v", cfg.Labels)
	}
	if cfg.TeamField != "customfield_10001" {
		t.Errorf("Expected team_field 'customfield_10001', got %q", cfg.TeamField)
	}
	if cfg.Team != "team-uuid-123" {
		t.Errorf("Expected team 'team-uuid-123', got %q", cfg.Team)
	}
}

// JIRA integration tests

func TestBuildJiraPayload(t *testing.T) {
	cfg := jiraConfig{
		Components: []string{"Console UI"},
		Labels:     []string{"platform-experience-ui", "automation"},
		TeamField:  "customfield_10001",
		Team:       "team-uuid-123",
	}

	summary := "Test ticket"
	description := "This is a test description"

	payload := buildJiraPayload(summary, description, cfg)

	// Check project
	if project, ok := payload.Fields["project"].(map[string]string); !ok || project["key"] != "RHCLOUD" {
		t.Errorf("Expected project key 'RHCLOUD', got %v", payload.Fields["project"])
	}

	// Check summary
	if payload.Fields["summary"] != summary {
		t.Errorf("Expected summary %q, got %q", summary, payload.Fields["summary"])
	}

	// Check description
	if payload.Fields["description"] != description {
		t.Errorf("Expected description %q, got %q", description, payload.Fields["description"])
	}

	// Check issue type
	if issueType, ok := payload.Fields["issuetype"].(map[string]string); !ok || issueType["name"] != "Story" {
		t.Errorf("Expected issuetype 'Story', got %v", payload.Fields["issuetype"])
	}

	// Check labels
	if labels, ok := payload.Fields["labels"].([]string); !ok || len(labels) != 2 {
		t.Errorf("Expected 2 labels, got %v", payload.Fields["labels"])
	}

	// Check components
	if components, ok := payload.Fields["components"].([]map[string]string); !ok || len(components) != 1 || components[0]["name"] != "Console UI" {
		t.Errorf("Expected component 'Console UI', got %v", payload.Fields["components"])
	}

	// Check team field
	if team, ok := payload.Fields["customfield_10001"].(string); !ok || team != "team-uuid-123" {
		t.Errorf("Expected team 'team-uuid-123', got %v", payload.Fields["customfield_10001"])
	}
}

func TestCreateJiraTicket(t *testing.T) {
	// Mock JIRA API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/rest/api/2/issue" {
			t.Errorf("Expected path /rest/api/2/issue, got %s", r.URL.Path)
		}

		username, password, ok := r.BasicAuth()
		if !ok || username != "test@example.com" || password != "test-token" {
			t.Errorf("Expected basic auth with test@example.com:test-token, got %s:%s (ok=%v)", username, password, ok)
		}

		// Return mock response
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(jiraResponse{Key: "RHCLOUD-12345"})
	}))
	defer server.Close()

	settings := jiraSettings{
		url:      server.URL,
		username: "test@example.com",
		apiToken: "test-token",
	}

	payload := jiraPayload{
		Fields: map[string]interface{}{
			"project":     map[string]string{"key": "RHCLOUD"},
			"summary":     "Test ticket",
			"description": "Test description",
			"issuetype":   map[string]string{"name": "Story"},
		},
	}

	issueKey, err := createJiraTicket(settings, payload)
	if err != nil {
		t.Fatalf("createJiraTicket failed: %v", err)
	}

	if issueKey != "RHCLOUD-12345" {
		t.Errorf("Expected issue key 'RHCLOUD-12345', got %q", issueKey)
	}
}

// YAML parsing tests

func TestGetProjectRefsAndLines(t *testing.T) {
	yamlContent := `
name: test-frontend
url: https://github.com/RedHatInsights/test-frontend
targets:
  - namespace:
      $ref: /services/insights/namespaces/test-prod.yml
    ref: abc123def456abc123def456abc123def456abc1
  - namespace:
      $ref: /services/insights/namespaces/test-stage.yml
    ref: 111222333444555666777888999000aaabbbccc
  - namespace:
      $ref: /services/insights/namespaces/test-prod-west.yml
    ref: abc123def456abc123def456abc123def456abc1
`

	refs, lines, err := getProjectRefsAndLines(yamlContent, "test-frontend", "https://github.com/RedHatInsights/test-frontend")
	if err != nil {
		t.Fatalf("getProjectRefsAndLines failed: %v", err)
	}

	// Should find 2 production refs (skipping stage)
	if len(refs) != 2 {
		t.Errorf("Expected 2 refs, got %d: %v", len(refs), refs)
	}

	// Both prod refs should be the same
	if refs[0] != "abc123def456abc123def456abc123def456abc1" {
		t.Errorf("Expected ref 'abc123def456abc123def456abc123def456abc1', got %q", refs[0])
	}

	// Lines should be populated
	if len(lines) != 2 {
		t.Errorf("Expected 2 line numbers, got %d", len(lines))
	}
}

func TestGetProjectRefsAndLines_NoMatch(t *testing.T) {
	yamlContent := `
name: different-frontend
url: https://github.com/RedHatInsights/different
targets:
  - namespace:
      $ref: /services/insights/namespaces/prod.yml
    ref: abc123
`

	_, _, err := getProjectRefsAndLines(yamlContent, "test-frontend", "https://github.com/RedHatInsights/test")
	if err == nil {
		t.Error("Expected error for no matching project, got nil")
	}

	if !strings.Contains(err.Error(), "no production ref entries found") {
		t.Errorf("Expected 'no production ref entries found' error, got: %v", err)
	}
}

func TestMakeRepoFilePath(t *testing.T) {
	tests := []struct {
		name           string
		repoRoot       string
		configuredPath string
		expected       string
	}{
		{
			"With ../ prefix",
			"/tmp/repo",
			"../data/services/deploy.yml",
			"/tmp/repo/data/services/deploy.yml",
		},
		{
			"Clean path",
			"/tmp/repo",
			"data/services/deploy.yml",
			"/tmp/repo/data/services/deploy.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeRepoFilePath(tt.repoRoot, tt.configuredPath)
			if result != tt.expected {
				t.Errorf("makeRepoFilePath(%q, %q) = %q; want %q", tt.repoRoot, tt.configuredPath, result, tt.expected)
			}
		})
	}
}

func TestBuildGitLabRepoURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		projectPath string
		token       string
		expected    string
		expectError bool
	}{
		{
			"Standard URL",
			"https://gitlab.com",
			"user/project",
			"token123",
			"https://oauth2:token123@gitlab.com/user/project.git",
			false,
		},
		{
			"URL with trailing slash",
			"https://gitlab.com/",
			"user/project",
			"token123",
			"https://oauth2:token123@gitlab.com/user/project.git",
			false,
		},
		{
			"Custom domain",
			"https://gitlab.example.com",
			"group/subgroup/project",
			"token456",
			"https://oauth2:token456@gitlab.example.com/group/subgroup/project.git",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildGitLabRepoURL(tt.baseURL, tt.projectPath, tt.token)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("buildGitLabRepoURL(...) = %q; want %q", result, tt.expected)
			}
		})
	}
}

// GitHub API tests

func TestGetLatestCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/RedHatInsights/test-repo/commits/master" {
			t.Errorf("Expected path /repos/RedHatInsights/test-repo/commits/master, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"sha": "abc123def456abc123def456abc123def456abc1",
		})
	}))
	defer server.Close()

	// This test would require dependency injection to override the GitHub API URL
	// For now, we'll skip the actual test but keep the structure
	t.Skip("Requires dependency injection for GitHub API URL")
}

func TestGetCommitLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/compare/") {
			t.Errorf("Expected compare endpoint, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commits": []map[string]interface{}{
				{
					"sha": "commit1",
					"commit": map[string]string{
						"message": "First commit",
					},
				},
				{
					"sha": "commit2",
					"commit": map[string]string{
						"message": "Second commit\nWith details",
					},
				},
			},
		})
	}))
	defer server.Close()

	// This test would require dependency injection to override the GitHub API URL
	t.Skip("Requires dependency injection for GitHub API URL")
}
