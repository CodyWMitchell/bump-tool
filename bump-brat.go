package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

const (
	configFile   = "bump-brat-config.yml"
	colorMagenta = "201" // Magenta for emphasis and warnings
	colorGreen   = "118" // Lime green for success and highlights
)

// UI Styles using our lime-green and magenta color scheme
var (
	titleStyle         = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color(colorMagenta))
	upToDateStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	updateStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	magentaStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	greenStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	sectionStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorMagenta)).MarginTop(1)
	stepStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	okStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	progressFillStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	progressEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta))
	mrBoxStyle         = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorMagenta)).
				Padding(0, 1).
				MarginTop(1)
	shaPattern     = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
	percentPattern = regexp.MustCompile(`(\d{1,3})%`)
)

// Configuration structures
type config struct {
	Projects []projectConfig `yaml:"projects"`
}

type projectConfig struct {
	Name     string `yaml:"name"`
	FilePath string `yaml:"file_path"`
	RepoURL  string `yaml:"repo_url"`
}

func loadConfig() (config, error) {
	var cfg config

	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Data models for projects and commits
type commit struct {
	sha     string
	message string
	repoURL string
}

func (c commit) Title() string {
	return fmt.Sprintf("%s - %s", greenStyle.Render(c.sha[:7]), c.message)
}

func (c commit) Description() string { return "" }
func (c commit) FilterValue() string { return c.message }

type project struct {
	name                 string
	filePath             string
	repoURL              string
	currentRef           string
	latestRef            string
	commitCount          int
	commits              []commit
	commitLogUnavailable bool
	commitLogWarning     string
}

func (p project) needsUpdate() bool {
	return p.currentRef != p.latestRef
}

func (p project) Title() string {
	status := ""
	if p.currentRef == "" || p.latestRef == "" {
		status = "⟳ checking..."
	} else if p.needsUpdate() {
		commitInfo := ""
		if p.commitCount > 0 {
			commitText := "commit"
			if p.commitCount > 1 {
				commitText = "commits"
			}
			commitInfo = fmt.Sprintf(" (%d %s)", p.commitCount, commitText)
		} else if p.commitLogUnavailable {
			commitInfo = " (commit list unavailable)"
		}
		status = updateStyle.Render(fmt.Sprintf("→ %s → %s%s", shortSHA(p.currentRef), shortSHA(p.latestRef), commitInfo))
	} else {
		status = upToDateStyle.Render(fmt.Sprintf("✓ up to date (%s)", shortSHA(p.currentRef)))
	}

	return fmt.Sprintf("%s %s", p.name, status)
}

func (p project) Description() string { return "" }
func (p project) FilterValue() string { return p.name }

// Bubble Tea TUI model
type viewMode int

const (
	viewList viewMode = iota
	viewDetail
)

type model struct {
	list            list.Model
	commitList      list.Model
	projects        []project
	loading         bool
	err             error
	quitting        bool
	ready           bool
	mode            viewMode
	detailProject   *project
	selectedProject *project
}

type gitLabSettings struct {
	baseURL         string
	token           string
	forkProject     string
	upstreamProject string
	targetBranch    string
}

type projectsLoadedMsg struct {
	projects []project
	err      error
}

func initialModel() (model, error) {
	cfg, err := loadConfig()
	if err != nil {
		return model{}, err
	}

	projects := make([]project, len(cfg.Projects))
	for i, p := range cfg.Projects {
		projects[i] = project{
			name:     p.Name,
			filePath: p.FilePath,
			repoURL:  p.RepoURL,
		}
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].name < projects[j].name
	})

	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = p
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Bump Brat - Frontend Production Hash Bumper"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle

	return model{
		list:     l,
		projects: projects,
		loading:  true,
		mode:     viewList,
	}, nil
}

func loadProjectStatus(projects []project) tea.Cmd {
	return func() tea.Msg {
		settings, err := loadGitLabSettings()
		if err != nil {
			return projectsLoadedMsg{err: err}
		}

		upstreamProjectID, err := getGitLabProjectID(settings, settings.upstreamProject)
		if err != nil {
			return projectsLoadedMsg{err: fmt.Errorf("failed to resolve upstream app-interface project: %w", err)}
		}

		for i := range projects {
			// Get current deployed ref from upstream app-interface.
			currentRef, err := getCurrentRefFromGitLab(settings, upstreamProjectID, projects[i].filePath, projects[i].name, projects[i].repoURL)
			if err != nil {
				return projectsLoadedMsg{err: fmt.Errorf("%s: failed to get current ref: %w", projects[i].name, err)}
			}
			projects[i].currentRef = currentRef

			// Get latest ref from GitHub
			latestRef, err := getLatestCommit(projects[i].repoURL)
			if err != nil {
				return projectsLoadedMsg{err: fmt.Errorf("%s: failed to get latest commit from GitHub: %w", projects[i].name, err)}
			}
			projects[i].latestRef = latestRef

			// Get commits if there's an update
			if currentRef != latestRef {
				commits, err := getCommitLog(projects[i].repoURL, currentRef, latestRef)
				if err != nil {
					projects[i].commitLogUnavailable = true
					projects[i].commitLogWarning = err.Error()
					projects[i].commits = nil
					projects[i].commitCount = 0
					continue
				}
				projects[i].commits = commits
				projects[i].commitCount = len(commits)
			}
		}
		return projectsLoadedMsg{projects: projects}
	}
}

func (m model) Init() tea.Cmd {
	return loadProjectStatus(m.projects)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		if m.mode == viewDetail {
			m.commitList.SetWidth(msg.Width)
			m.commitList.SetHeight(msg.Height - 4)
		}
		m.ready = true
		return m, nil

	case projectsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.projects = msg.projects
		items := make([]list.Item, len(m.projects))
		for i, p := range m.projects {
			items[i] = p
		}
		m.list.SetItems(items)
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch m.mode {
		case viewList:
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit

			case "d":
				// Show detail view for current project
				if i := m.list.Index(); i >= 0 && i < len(m.projects) {
					p := &m.projects[i]
					if p.needsUpdate() && len(p.commits) > 0 {
						m.detailProject = p
						m.mode = viewDetail

						// Create commit list
						items := make([]list.Item, len(p.commits))
						for j, c := range p.commits {
							items[j] = c
						}
						m.commitList = list.New(items, list.NewDefaultDelegate(), 0, 0)
						m.commitList.Title = fmt.Sprintf("%s - Commits", p.name)
						m.commitList.SetShowStatusBar(false)
						m.commitList.SetFilteringEnabled(false)
						m.commitList.Styles.Title = titleStyle
						if m.ready {
							m.commitList.SetWidth(m.list.Width())
							m.commitList.SetHeight(m.list.Height())
						}
					}
				}
				return m, nil

			case "enter":
				// Select current project for processing
				if i := m.list.Index(); i >= 0 && i < len(m.projects) {
					p := m.projects[i]
					if p.needsUpdate() {
						m.selectedProject = &p
						m.quitting = true
						return m, tea.Quit
					}
				}
				return m, nil
			}

		case viewDetail:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				// Go back to list view
				m.mode = viewList
				m.detailProject = nil
				return m, nil

			case "enter":
				// Open commit in browser
				if i := m.commitList.Index(); i >= 0 && i < len(m.detailProject.commits) {
					c := m.detailProject.commits[i]
					url := fmt.Sprintf("%s/commit/%s", c.repoURL, c.sha)
					exec.Command("open", url).Start()
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.mode {
	case viewList:
		m.list, cmd = m.list.Update(msg)
	case viewDetail:
		m.commitList, cmd = m.commitList.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v\n", m.err))
	}

	if m.quitting {
		return ""
	}

	if !m.ready {
		return "Initializing..."
	}

	var help string
	var content string

	switch m.mode {
	case viewList:
		help = helpStyle.Render("\n  d: view details • enter: bump selected project • q: quit")
		content = m.list.View()
	case viewDetail:
		help = helpStyle.Render("\n  enter: open commit on GitHub • esc: back to list • q: quit")
		content = m.commitList.View()
	}

	return content + help
}

// Git and GitHub API functions
func getCurrentRef(filePath string, lineNum int) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return "", fmt.Errorf("line %d is out of range for %s", lineNum, filePath)
	}

	line := strings.TrimSpace(lines[lineNum-1])
	ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
	if ref == "" {
		return "", fmt.Errorf("line %d in %s does not contain a ref value", lineNum, filePath)
	}
	return ref, nil
}

func makeGitHubRequest(apiURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub token if available
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	}

	return http.DefaultClient.Do(req)
}

func shortSHA(ref string) string {
	if len(ref) <= 7 {
		return ref
	}
	return ref[:7]
}

func envBool(name string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func buildAISummaryPrompt(p project, commits []commit) string {
	var b strings.Builder
	b.WriteString("You are writing a concise release blurb for engineering stakeholders.\n")
	b.WriteString("Return exactly 1-2 sentences and no bullet list.\n")
	b.WriteString("Mention the project and summarize what changed across these commits.\n\n")
	b.WriteString(fmt.Sprintf("Project: %s\n", p.name))
	b.WriteString(fmt.Sprintf("From: %s\n", p.currentRef))
	b.WriteString(fmt.Sprintf("To: %s\n", p.latestRef))
	b.WriteString("Commits:\n")
	for _, c := range commits {
		b.WriteString(fmt.Sprintf("- %s (%s)\n", c.message, shortSHA(c.sha)))
	}
	return b.String()
}

func generateClaudeAISummary(p project, commits []commit) (string, error) {
	if !envBool("BUMP_BRAT_USE_CLAUDE_SUMMARY") {
		return "", nil
	}
	if len(commits) == 0 {
		return "", nil
	}

	sessionID := strings.TrimSpace(os.Getenv("BUMP_BRAT_CLAUDE_SESSION_ID"))
	claudeBin := strings.TrimSpace(os.Getenv("BUMP_BRAT_CLAUDE_BIN"))
	if claudeBin == "" {
		claudeBin = "claude"
	}
	timeoutSeconds := envInt("BUMP_BRAT_CLAUDE_SUMMARY_TIMEOUT_SECONDS", 60)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	prompt := buildAISummaryPrompt(p, commits)
	args := []string{"-p", prompt}
	if sessionID != "" {
		args = []string{"--resume", sessionID, "-p", prompt}
	}
	cmd := exec.CommandContext(ctx, claudeBin, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("claude summary timed out after %d seconds", timeoutSeconds)
	}
	if err != nil {
		return "", fmt.Errorf("failed to run Claude summary command: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	summary := strings.TrimSpace(string(output))
	if summary == "" {
		return "", fmt.Errorf("claude summary output was empty")
	}

	// Keep MR/JIRA blurbs intentionally short.
	if len(summary) > 500 {
		summary = summary[:500]
	}
	return summary, nil
}

func normalizeRepoURL(repoURL string) string {
	return strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
}

func githubCommitURL(repoURL, sha string) string {
	return fmt.Sprintf("%s/commit/%s", normalizeRepoURL(repoURL), sha)
}

func getLatestCommit(repoURL string) (string, error) {
	repoName := strings.TrimPrefix(repoURL, "https://github.com/")
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/commits/master", repoName)

	resp, err := makeGitHubRequest(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w (body: %s)", err, string(body))
	}

	return result.SHA, nil
}

func getCommitLog(repoURL, oldRef, newRef string) ([]commit, error) {
	repoName := strings.TrimPrefix(repoURL, "https://github.com/")
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/compare/%s...%s", repoName, oldRef, newRef)

	resp, err := makeGitHubRequest(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("compare API unavailable for %s...%s", shortSHA(oldRef), shortSHA(newRef))
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	var commits []commit
	for _, c := range result.Commits {
		msg := strings.Split(c.Commit.Message, "\n")[0]
		commits = append(commits, commit{
			sha:     c.SHA,
			message: msg,
			repoURL: repoURL,
		})
	}

	return commits, nil
}

func updateRef(filePath string, lineNum int, newRef string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line %d is out of range for %s", lineNum, filePath)
	}

	current := lines[lineNum-1]
	idx := strings.Index(current, "ref:")
	if idx == -1 {
		return fmt.Errorf("line %d in %s does not contain ref:", lineNum, filePath)
	}
	lines[lineNum-1] = fmt.Sprintf("%sref: %s", current[:idx], newRef)
	updated := strings.Join(lines, "\n")
	return os.WriteFile(filePath, []byte(updated), 0o644)
}

func loadGitLabSettings() (gitLabSettings, error) {
	baseURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("GITLAB_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}

	settings := gitLabSettings{
		baseURL:         baseURL,
		token:           strings.TrimSpace(os.Getenv("GITLAB_TOKEN")),
		forkProject:     strings.TrimSpace(os.Getenv("APP_INTERFACE_FORK_PROJECT")),
		upstreamProject: strings.TrimSpace(os.Getenv("APP_INTERFACE_UPSTREAM_PROJECT")),
		targetBranch:    strings.TrimSpace(os.Getenv("APP_INTERFACE_TARGET_BRANCH")),
	}

	if settings.token == "" {
		return settings, fmt.Errorf("GITLAB_TOKEN is required")
	}
	if settings.forkProject == "" {
		return settings, fmt.Errorf("APP_INTERFACE_FORK_PROJECT is required")
	}
	if settings.upstreamProject == "" {
		return settings, fmt.Errorf("APP_INTERFACE_UPSTREAM_PROJECT is required")
	}
	if settings.targetBranch == "" {
		settings.targetBranch = "master"
	}

	return settings, nil
}

func makeRepoFilePath(repoRoot, configuredPath string) string {
	return filepath.Join(repoRoot, normalizedProjectFilePath(configuredPath))
}

func buildGitLabRepoURL(baseURL, projectPath, token string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid GITLAB_BASE_URL: %w", err)
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	parsed.User = url.UserPassword("oauth2", token)
	return fmt.Sprintf("%s/%s.git", strings.TrimSuffix(parsed.String(), "/"), projectPath), nil
}

func normalizedProjectFilePath(configuredPath string) string {
	clean := strings.TrimSpace(configuredPath)
	for strings.HasPrefix(clean, "../") {
		clean = strings.TrimPrefix(clean, "../")
	}
	return strings.TrimPrefix(clean, "./")
}

func mappingValueByKey(node *yaml.Node, keyName string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Kind == yaml.ScalarNode && key.Value == keyName {
			return value
		}
	}
	return nil
}

func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func isProductionNamespaceRef(namespaceRef string) bool {
	ref := strings.ToLower(strings.TrimSpace(namespaceRef))
	if ref == "" {
		return false
	}
	return strings.Contains(ref, "prod") && !strings.Contains(ref, "stage") && !strings.Contains(ref, "ephemeral")
}

func collectRefsFromTargets(targets *yaml.Node, refs *[]string, lines *[]int, seen map[int]bool) {
	if targets == nil || targets.Kind != yaml.SequenceNode {
		return
	}

	for _, targetItem := range targets.Content {
		if targetItem.Kind != yaml.MappingNode {
			continue
		}
		namespaceNode := mappingValueByKey(targetItem, "namespace")
		refNode := mappingValueByKey(targetItem, "ref")
		namespaceRef := scalarValue(mappingValueByKey(namespaceNode, "$ref"))
		refValue := scalarValue(refNode)

		if !isProductionNamespaceRef(namespaceRef) || !shaPattern.MatchString(refValue) || refNode == nil {
			continue
		}
		if seen[refNode.Line] {
			continue
		}

		*refs = append(*refs, refValue)
		*lines = append(*lines, refNode.Line)
		seen[refNode.Line] = true
	}
}

func collectProjectRefsAndLines(node *yaml.Node, projectName, repoURL string, refs *[]string, lines *[]int, seen map[int]bool) {
	if node == nil {
		return
	}

	if node.Kind == yaml.MappingNode {
		name := scalarValue(mappingValueByKey(node, "name"))
		url := scalarValue(mappingValueByKey(node, "url"))
		matchedByURL := repoURL != "" && strings.EqualFold(strings.TrimSpace(url), strings.TrimSpace(repoURL))
		matchedByName := projectName != "" && strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(projectName))
		if matchedByURL || matchedByName {
			collectRefsFromTargets(mappingValueByKey(node, "targets"), refs, lines, seen)
		}
	}

	for _, child := range node.Content {
		collectProjectRefsAndLines(child, projectName, repoURL, refs, lines, seen)
	}
}

func getProjectRefsAndLines(raw, projectName, repoURL string) ([]string, []int, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse deployment YAML: %w", err)
	}

	refs := make([]string, 0, 2)
	lines := make([]int, 0, 2)
	seen := map[int]bool{}
	collectProjectRefsAndLines(&root, projectName, repoURL, &refs, &lines, seen)

	if len(refs) == 0 {
		if repoURL != "" {
			return nil, nil, fmt.Errorf("no production ref entries found for project %q with repo %q", projectName, repoURL)
		}
		return nil, nil, fmt.Errorf("no production ref entries found for project %q", projectName)
	}

	return refs, lines, nil
}

func getCurrentRefFromGitLab(settings gitLabSettings, projectID int, configuredFilePath, projectName, repoURL string) (string, error) {
	filePath := normalizedProjectFilePath(configuredFilePath)
	escapedPath := url.PathEscape(filePath)
	refQuery := url.QueryEscape(settings.targetBranch)
	endpoint := fmt.Sprintf("/projects/%d/repository/files/%s?ref=%s", projectID, escapedPath, refQuery)

	body, err := gitLabAPIRequest(settings, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	var fileResp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileResp.Content, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("failed to decode file content for %s: %w", filePath, err)
	}

	refs, _, err := getProjectRefsAndLines(string(decoded), projectName, repoURL)
	if err != nil {
		return "", fmt.Errorf("%s: %w", filePath, err)
	}

	// Frontend refs are expected to be duplicated for prod namespaces; first one is enough for compare.
	return refs[0], nil
}

func updateProjectRefsInFile(filePath, projectName, repoURL, newRef string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	_, lines, err := getProjectRefsAndLines(string(data), projectName, repoURL)
	if err != nil {
		return err
	}

	for _, lineNum := range lines {
		if err := updateRef(filePath, lineNum, newRef); err != nil {
			return err
		}
	}
	return nil
}

func runGitWithLiveProgress(repoDir, step string, args ...string) error {
	fmt.Printf("   %s %s...\n", stepStyle.Render("→"), stepStyle.Render(step))
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to open git stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start git command: %w", err)
	}

	showedProgress := false
	scanner := bufio.NewScanner(stderrPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanner.Split(splitCRLF)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		phase, percent, ok := parseGitProgress(line)
		if ok {
			showedProgress = true
			bar := renderProgressBar(percent, 26)
			fmt.Printf("\r     %s %3d%% %s", bar, percent, helpStyle.Render(phase))
			continue
		}

		if showedProgress {
			fmt.Print("\n")
			showedProgress = false
		}
		fmt.Printf("     %s\n", line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed reading git progress output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	if showedProgress {
		fmt.Print("\n")
	}
	fmt.Printf("   %s %s\n", okStyle.Render("✓"), okStyle.Render(step))
	return nil
}

func splitCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, bytes.TrimSpace(data[:i]), nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), bytes.TrimSpace(data), nil
	}
	return 0, nil, nil
}

func parseGitProgress(line string) (string, int, bool) {
	match := percentPattern.FindStringSubmatch(line)
	if len(match) < 2 {
		return "", 0, false
	}
	percent, err := strconv.Atoi(match[1])
	if err != nil {
		return "", 0, false
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "receiving objects"):
		return "receiving objects", percent, true
	case strings.Contains(lower, "counting objects"):
		return "counting objects", percent, true
	case strings.Contains(lower, "compressing objects"):
		return "compressing objects", percent, true
	case strings.Contains(lower, "resolving deltas"):
		return "resolving deltas", percent, true
	case strings.Contains(lower, "writing objects"):
		return "writing objects", percent, true
	default:
		return "git progress", percent, true
	}
}

func renderProgressBar(percent, width int) string {
	if width <= 0 {
		width = 20
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := (percent * width) / 100
	if filled > width {
		filled = width
	}
	empty := width - filled
	fill := progressFillStyle.Render(strings.Repeat("█", filled))
	rest := progressEmptyStyle.Render(strings.Repeat("░", empty))
	return "[" + fill + rest + "]"
}

func gitLabAPIRequest(settings gitLabSettings, method, path string, body any) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v4%s", settings.baseURL, path)

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewBuffer(raw)
	}

	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", settings.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &gitLabAPIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}
	return respBody, nil
}

type gitLabAPIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *gitLabAPIError) Error() string {
	return fmt.Sprintf("GitLab API %s %s failed with status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func isGitLabStatusError(err error, status int) bool {
	var apiErr *gitLabAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

func getGitLabProjectID(settings gitLabSettings, projectPath string) (int, error) {
	encoded := url.PathEscape(projectPath)
	body, err := gitLabAPIRequest(settings, "GET", fmt.Sprintf("/projects/%s", encoded), nil)
	if err != nil {
		return 0, err
	}
	var project struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &project); err != nil {
		return 0, err
	}
	return project.ID, nil
}

func createGitLabMR(settings gitLabSettings, upstreamID, forkID int, sourceBranch, title, description string) (string, error) {
	upstreamPayload := map[string]any{
		"source_branch":        sourceBranch,
		"target_branch":        settings.targetBranch,
		"source_project_id":    forkID,
		"title":                title,
		"description":          description,
		"remove_source_branch": true,
	}

	body, err := gitLabAPIRequest(settings, "POST", fmt.Sprintf("/projects/%d/merge_requests", upstreamID), upstreamPayload)
	if err != nil && (isGitLabStatusError(err, 403) || isGitLabStatusError(err, 404)) {
		// Some GitLab instances require creating the MR from the fork project endpoint.
		forkPayload := map[string]any{
			"source_branch":        sourceBranch,
			"target_branch":        settings.targetBranch,
			"target_project_id":    upstreamID,
			"title":                title,
			"description":          description,
			"remove_source_branch": true,
		}
		body, err = gitLabAPIRequest(settings, "POST", fmt.Sprintf("/projects/%d/merge_requests", forkID), forkPayload)
	}
	if err != nil {
		if isGitLabStatusError(err, 403) {
			return "", fmt.Errorf("%w (ensure token has api scope and your user can create merge requests in %s)", err, settings.upstreamProject)
		}
		return "", err
	}

	var mr struct {
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(body, &mr); err != nil {
		return "", err
	}
	if mr.WebURL == "" {
		return "", fmt.Errorf("GitLab API did not return merge request URL")
	}
	return mr.WebURL, nil
}

func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Keep already exported values, but populate from .env when missing.
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// Project processing and output
func processProject(p project) error {
	settings, err := loadGitLabSettings()
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(sectionStyle.Render("Processing project"))
	fmt.Println()

	fmt.Printf("📦 %s\n", magentaStyle.Render(p.name))
	fmt.Printf("   Current: %s\n", updateStyle.Render(shortSHA(p.currentRef)))
	fmt.Printf("   Latest:  %s\n", greenStyle.Render(shortSHA(p.latestRef)))

	// Use already fetched commits
	commits := p.commits
	if len(commits) == 0 && !p.commitLogUnavailable {
		// Fetch if not already loaded
		commits, err = getCommitLog(p.repoURL, p.currentRef, p.latestRef)
		if err != nil {
			p.commitLogUnavailable = true
			p.commitLogWarning = err.Error()
			commits = nil
		}
	}

	if len(commits) > 0 {
		fmt.Println("   Commits:")
		for _, c := range commits {
			fmt.Printf("     - %s (%s)\n", c.message, shortSHA(c.sha))
		}
	} else if p.commitLogUnavailable {
		fmt.Printf("   %s %s\n", updateStyle.Render("Warning:"), p.commitLogWarning)
	}

	repoDir, err := os.MkdirTemp("", "bump-brat-app-interface-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(repoDir)

	forkRepoURL, err := buildGitLabRepoURL(settings.baseURL, settings.forkProject, settings.token)
	if err != nil {
		return err
	}
	upstreamRepoURL, err := buildGitLabRepoURL(settings.baseURL, settings.upstreamProject, settings.token)
	if err != nil {
		return err
	}

	fmt.Println(sectionStyle.Render("Syncing app-interface fork with upstream"))
	if err := runGitWithLiveProgress("", "cloning fork", "clone", "--progress", forkRepoURL, repoDir); err != nil {
		return err
	}
	if err := runGitWithLiveProgress(repoDir, "adding upstream remote", "remote", "add", "upstream", upstreamRepoURL); err != nil {
		return err
	}
	if err := runGitWithLiveProgress(repoDir, "fetching upstream branch", "fetch", "--progress", "upstream", settings.targetBranch); err != nil {
		return err
	}
	if err := runGitWithLiveProgress(repoDir, "checking out synced base branch", "checkout", "-B", settings.targetBranch, fmt.Sprintf("upstream/%s", settings.targetBranch)); err != nil {
		return err
	}

	branchName := fmt.Sprintf("bump/%s-%s-%d", p.name, shortSHA(p.latestRef), time.Now().Unix())
	if err := runGitWithLiveProgress(repoDir, "creating bump branch", "checkout", "-b", branchName); err != nil {
		return err
	}

	repoFilePath := makeRepoFilePath(repoDir, p.filePath)
	if _, err := os.Stat(repoFilePath); err != nil {
		return fmt.Errorf("deployment file not found in cloned app-interface repo: %s", repoFilePath)
	}

	// Update all matching refs for this frontend by name in cloned app-interface checkout.
	if err := updateProjectRefsInFile(repoFilePath, p.name, p.repoURL, p.latestRef); err != nil {
		return fmt.Errorf("failed to update ref in %s: %w", repoFilePath, err)
	}

	fmt.Printf("   %s\n\n", okStyle.Render("✓ Updated refs"))

	// Stage file in cloned app-interface repo.
	fmt.Println(sectionStyle.Render("Staging and committing changes"))
	if err := runGitWithLiveProgress(repoDir, "staging deployment changes", "add", repoFilePath); err != nil {
		return err
	}
	if err := runGitWithLiveProgress(repoDir, "creating commit",
		"-c", "user.name=bump-brat",
		"-c", "user.email=bump-brat@local",
		"commit", "-m", fmt.Sprintf("%s: bump to %s", p.name, shortSHA(p.latestRef)),
	); err != nil {
		return err
	}
	if err := runGitWithLiveProgress(repoDir, "pushing branch to fork", "push", "--progress", "-u", "origin", branchName); err != nil {
		return err
	}
	fmt.Printf("   %s\n\n", okStyle.Render("✓ Branch pushed"))

	title := fmt.Sprintf("%s: bump to %s", p.name, shortSHA(p.latestRef))
	oldHashLink := fmt.Sprintf("[%s](%s)", shortSHA(p.currentRef), githubCommitURL(p.repoURL, p.currentRef))
	newHashLink := fmt.Sprintf("[%s](%s)", shortSHA(p.latestRef), githubCommitURL(p.repoURL, p.latestRef))
	description := fmt.Sprintf("Bumps %s from %s to %s\n\n", p.name, oldHashLink, newHashLink)
	aiSummary, err := generateClaudeAISummary(p, commits)
	if err != nil {
		fmt.Printf("   %s AI summary unavailable: %v\n", updateStyle.Render("Warning:"), err)
	} else if aiSummary != "" {
		fmt.Println(sectionStyle.Render("AI summary"))
		fmt.Printf("   %s\n\n", aiSummary)
		description += fmt.Sprintf("### AI summary\n%s\n\n", aiSummary)
	}
	description += "### Included commits\n"
	for _, c := range commits {
		commitLink := fmt.Sprintf("[%s](%s)", shortSHA(c.sha), githubCommitURL(c.repoURL, c.sha))
		description += fmt.Sprintf("- %s (%s)\n", c.message, commitLink)
	}
	if p.commitLogUnavailable {
		description += fmt.Sprintf("- Commit list unavailable: %s\n", p.commitLogWarning)
	}
	description += "\n---\n🤖 Generated by bump-brat"

	upstreamID, err := getGitLabProjectID(settings, settings.upstreamProject)
	if err != nil {
		return err
	}
	forkID, err := getGitLabProjectID(settings, settings.forkProject)
	if err != nil {
		return err
	}
	mrURL, err := createGitLabMR(settings, upstreamID, forkID, branchName, title, description)
	if err != nil {
		return err
	}

	mrSummary := strings.Join([]string{
		magentaStyle.Render("📋 Merge Request Created"),
		"",
		fmt.Sprintf("Project: %s", settings.upstreamProject),
		fmt.Sprintf("Branch:  %s", branchName),
		fmt.Sprintf("MR URL:  %s", mrURL),
	}, "\n")
	fmt.Println(mrBoxStyle.Render(mrSummary))

	fmt.Println(greenStyle.Render("\n✓ Done!"))
	fmt.Println(helpStyle.Render("Fork synced, branch pushed, and MR opened in GitLab."))

	return nil
}

func main() {
	if err := loadDotEnv(".env"); err != nil {
		fmt.Printf("%s %v\n", errorStyle.Render("Error loading .env:"), err)
		os.Exit(1)
	}

	// Check for GitLab authentication and project config.
	if _, err := loadGitLabSettings(); err == nil {
		fmt.Println(greenStyle.Render("✓ GitLab automation enabled"))
	} else {
		fmt.Println(updateStyle.Render("⚠ GitLab automation config is incomplete"))
		fmt.Println(helpStyle.Render("  Set GITLAB_TOKEN, APP_INTERFACE_FORK_PROJECT, and APP_INTERFACE_UPSTREAM_PROJECT"))
		fmt.Printf("  %s\n", helpStyle.Render(err.Error()))
	}
	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println(helpStyle.Render("  Optional: set GITHUB_TOKEN to avoid GitHub API rate limits"))
	}
	if envBool("BUMP_BRAT_USE_CLAUDE_SUMMARY") {
		if strings.TrimSpace(os.Getenv("BUMP_BRAT_CLAUDE_SESSION_ID")) == "" {
			fmt.Println(greenStyle.Render("✓ Claude summary blurb enabled"))
			fmt.Println(helpStyle.Render("  Using default local Claude auth/session context"))
		} else {
			fmt.Println(greenStyle.Render("✓ Claude summary blurb enabled"))
			fmt.Println(helpStyle.Render("  Using configured resume session ID"))
		}
	}
	fmt.Println()

	initialModel, err := initialModel()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel, tea.WithAltScreen())

	m, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Process selected project if one was chosen
	if finalModel, ok := m.(model); ok {
		if finalModel.err != nil {
			fmt.Printf("\n%s\n", errorStyle.Render(fmt.Sprintf("Error: %v", finalModel.err)))
			os.Exit(1)
		}
		if finalModel.selectedProject != nil {
			if err := processProject(*finalModel.selectedProject); err != nil {
				fmt.Printf("\n%s\n", errorStyle.Render(fmt.Sprintf("Error: %v", err)))
				os.Exit(1)
			}
		}
	}
}
