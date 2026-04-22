package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"

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
	titleStyle    = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color(colorMagenta))
	upToDateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	updateStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	magentaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMagenta)).Bold(true)
	greenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
)

// Configuration structures
type config struct {
	Projects []projectConfig `yaml:"projects"`
}

type projectConfig struct {
	Name     string `yaml:"name"`
	FilePath string `yaml:"file_path"`
	RepoURL  string `yaml:"repo_url"`
	Line1    int    `yaml:"line1"`
	Line2    int    `yaml:"line2"`
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
	name        string
	filePath    string
	repoURL     string
	line1       int
	line2       int
	currentRef  string
	latestRef   string
	commitCount int
	commits     []commit
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
		}
		status = updateStyle.Render(fmt.Sprintf("→ %s → %s%s", p.currentRef[:7], p.latestRef[:7], commitInfo))
	} else {
		status = upToDateStyle.Render(fmt.Sprintf("✓ up to date (%s)", p.currentRef[:7]))
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
	list           list.Model
	commitList     list.Model
	projects       []project
	loading        bool
	err            error
	quitting       bool
	ready          bool
	mode           viewMode
	detailProject  *project
	selectedProject *project
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
			line1:    p.Line1,
			line2:    p.Line2,
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
		for i := range projects {
			// Get current ref
			currentRef, err := getCurrentRef(projects[i].filePath, projects[i].line1)
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
					return projectsLoadedMsg{err: fmt.Errorf("%s: failed to get commit log: %w", projects[i].name, err)}
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
	cmd := exec.Command("sed", "-n", fmt.Sprintf("%dp", lineNum), filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(output))
	ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
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
	sedExpr := fmt.Sprintf("%ds/ref: .*/ref: %s/", lineNum, newRef)
	cmd := exec.Command("sed", "-i", "", sedExpr, filePath)
	return cmd.Run()
}

// Project processing and output
func processProject(p project) error {
	fmt.Println("\n" + magentaStyle.Render("Processing project...") + "\n")

	fmt.Printf("📦 %s\n", magentaStyle.Render(p.name))
	fmt.Printf("   Current: %s\n", updateStyle.Render(p.currentRef[:7]))
	fmt.Printf("   Latest:  %s\n", greenStyle.Render(p.latestRef[:7]))

	// Use already fetched commits
	commits := p.commits
	if len(commits) == 0 {
		// Fetch if not already loaded
		var err error
		commits, err = getCommitLog(p.repoURL, p.currentRef, p.latestRef)
		if err != nil {
			return fmt.Errorf("failed to get commit log for %s: %w", p.name, err)
		}
	}

	fmt.Println("   Commits:")
	for _, c := range commits {
		fmt.Printf("     - %s (%s)\n", c.message, c.sha[:7])
	}

	// Update refs
	if err := updateRef(p.filePath, p.line1, p.latestRef); err != nil {
		return fmt.Errorf("failed to update ref in %s: %w", p.filePath, err)
	}
	if err := updateRef(p.filePath, p.line2, p.latestRef); err != nil {
		return fmt.Errorf("failed to update ref in %s: %w", p.filePath, err)
	}

	fmt.Printf("   %s\n\n", upToDateStyle.Render("✓ Updated"))

	// Stage file
	fmt.Println(magentaStyle.Render("Staging changes..."))
	cmd := exec.Command("git", "add", p.filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stage file: %w\nOutput: %s", err, string(output))
	}
	fmt.Printf("   %s\n\n", upToDateStyle.Render("✓ Changes staged"))

	// Create commit message
	commitMsg := fmt.Sprintf("%s: bump to %s", p.name, p.latestRef[:7])

	// Build MR body
	mrBody := fmt.Sprintf("Bumps %s from %s to %s\n\n", p.name, p.currentRef[:7], p.latestRef[:7])
	for _, c := range commits {
		mrBody += fmt.Sprintf("- %s (%s)\n", c.message, c.sha[:7])
	}
	mrBody += "\n---\n🤖 Generated by bump-brat"

	// Display commit message and MR body
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println(magentaStyle.Render("\n📋 COMMIT MESSAGE"))
	fmt.Println(helpStyle.Render("Copy and use with: git commit -m \"...\""))
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println(commitMsg)
	fmt.Println(strings.Repeat("-", 80))

	fmt.Println(magentaStyle.Render("\n📋 MERGE REQUEST BODY"))
	fmt.Println(helpStyle.Render("Copy and use when creating your MR"))
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println(mrBody)
	fmt.Println(strings.Repeat("-", 80))

	fmt.Println(greenStyle.Render("\n✓ Done!"))
	fmt.Println(helpStyle.Render("Changes have been staged. Review them with 'git diff --staged'"))

	return nil
}

func main() {
	// Check for GitHub authentication
	if os.Getenv("GITHUB_TOKEN") != "" {
		fmt.Println(greenStyle.Render("✓ GitHub authentication enabled"))
	} else {
		fmt.Println(updateStyle.Render("⚠ No GITHUB_TOKEN set - rate limit: 60 requests/hour"))
		fmt.Println(helpStyle.Render("  Set GITHUB_TOKEN env var for 5000 requests/hour"))
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
