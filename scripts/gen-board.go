// gen-board generates tracker/BOARD.md from issue files.
//
// Usage:
//
//	go run ./scripts/gen-board.go [--root <repo-root>]
//
// It reads all BRD-*.md files from tracker/{backlog,todo,in-progress,review,done}/,
// parses their YAML frontmatter, validates epic refs and depends_on links, then
// writes a fresh tracker/BOARD.md.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Issue holds the parsed frontmatter of one BRD-*.md file.
type Issue struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Epic      string   `yaml:"epic"`
	Status    string   `yaml:"status"`
	Priority  string   `yaml:"priority"`
	Estimate  string   `yaml:"estimate"`
	Assignee  string   `yaml:"assignee"`
	DependsOn []string `yaml:"depends_on"`
	Blocks    []string `yaml:"blocks"`
	Labels    []string `yaml:"labels"`

	// Set by the loader, not from frontmatter.
	FolderStatus string `yaml:"-"`
	FilePath     string `yaml:"-"`
}

var statusOrder = []string{"in-progress", "todo", "review", "done", "backlog"}

var statusLabel = map[string]string{
	"in-progress": "🔄 In Progress",
	"todo":        "📋 Todo",
	"review":      "👀 Review",
	"done":        "✅ Done",
	"backlog":     "📦 Backlog",
}

func main() {
	root := flag.String("root", ".", "repo root directory")
	flag.Parse()

	trackerDir := filepath.Join(*root, "tracker")

	issues, warnings := loadIssues(trackerDir)
	epicsFile := filepath.Join(trackerDir, "EPICS.md")
	epicWarnings := validateEpics(issues, epicsFile)
	warnings = append(warnings, epicWarnings...)

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "WARN: %s\n", w)
	}

	board := generateBoard(issues)

	outPath := filepath.Join(trackerDir, "BOARD.md")
	if err := os.WriteFile(outPath, []byte(board), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing BOARD.md: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d issues, %d warnings)\n", outPath, len(issues), len(warnings))
}

func loadIssues(trackerDir string) ([]*Issue, []string) {
	var issues []*Issue
	var warnings []string

	for _, status := range statusOrder {
		dir := filepath.Join(trackerDir, status)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // empty or missing status dir is fine
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".md") {
				continue
			}
			// Skip non-issue files (README.md, ISSUE_TEMPLATE.md, etc.)
			if !strings.HasPrefix(name, "BRD-") {
				continue
			}
			path := filepath.Join(dir, name)
			issue, err := parseIssue(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("parse %s: %v", path, err))
				continue
			}
			issue.FolderStatus = status
			issue.FilePath = path
			issues = append(issues, issue)
		}
	}

	// Cross-validate depends_on references.
	ids := make(map[string]bool, len(issues))
	for _, i := range issues {
		if i.ID != "" {
			ids[i.ID] = true
		}
	}
	for _, issue := range issues {
		for _, dep := range issue.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep != "" && !ids[dep] {
				warnings = append(warnings, fmt.Sprintf("%s: depends_on %q not found", issue.ID, dep))
			}
		}
	}

	return issues, warnings
}

// parseIssue extracts the YAML frontmatter (between the first two --- lines) from path.
func parseIssue(path string) (*Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var fm strings.Builder
	inFM := false
	delimiters := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			delimiters++
			if delimiters == 1 {
				inFM = true
				continue
			}
			if delimiters == 2 {
				break
			}
		}
		if inFM {
			fm.WriteString(line + "\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if delimiters < 2 {
		return nil, fmt.Errorf("no valid frontmatter found")
	}

	var issue Issue
	if err := yaml.Unmarshal([]byte(fm.String()), &issue); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return &issue, nil
}

// validateEpics checks that each issue's epic field appears in EPICS.md.
func validateEpics(issues []*Issue, epicsFile string) []string {
	data, err := os.ReadFile(epicsFile)
	if err != nil {
		return []string{fmt.Sprintf("cannot read %s: %v", epicsFile, err)}
	}
	content := string(data)

	var warnings []string
	for _, issue := range issues {
		if issue.Epic == "" {
			continue
		}
		// Look for the epic ID as a heading token, e.g. "## EP-01"
		if !strings.Contains(content, issue.Epic) {
			warnings = append(warnings, fmt.Sprintf("%s: epic %q not found in EPICS.md", issue.ID, issue.Epic))
		}
	}
	return warnings
}

func generateBoard(issues []*Issue) string {
	byStatus := make(map[string][]*Issue, len(statusOrder))
	for _, i := range issues {
		byStatus[i.FolderStatus] = append(byStatus[i.FolderStatus], i)
	}

	// Sort each column by ID lexicographically (BRD-001 < BRD-002 …).
	for _, group := range byStatus {
		sort.Slice(group, func(a, b int) bool {
			return group[a].ID < group[b].ID
		})
	}

	var b strings.Builder

	b.WriteString("# Bordo Board\n\n")
	b.WriteString(fmt.Sprintf("> Last updated: %s  \n", time.Now().UTC().Format("2006-01-02")))
	b.WriteString("> Auto-generated by `make board` — do not edit manually.\n\n")

	// Summary table
	b.WriteString("## Summary\n\n")
	b.WriteString("| Column | Count |\n|---|---|\n")
	for _, s := range statusOrder {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", statusLabel[s], len(byStatus[s])))
	}
	b.WriteString("\n")
	b.WriteString("**Current focus (MVP):** Build layer — EP-01 → EP-02 → EP-04 → BRD-008 demo  \n")
	b.WriteString("`new java service → scaffold → docker build → artifact in registry`\n\n---\n\n")

	// One section per column
	for _, s := range statusOrder {
		group := byStatus[s]
		b.WriteString(fmt.Sprintf("## %s (%d)\n\n", statusLabel[s], len(group)))
		if len(group) == 0 {
			b.WriteString("_Nothing here._\n\n")
			continue
		}
		b.WriteString("| ID | Title | Epic | Priority | Estimate | Assignee |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, i := range group {
			assignee := i.Assignee
			if assignee == "" {
				assignee = "—"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				i.ID, i.Title, i.Epic, i.Priority, i.Estimate, assignee))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("_To create an issue: copy `tracker/ISSUE_TEMPLATE.md`. See `tracker/README.md` for rules._\n")

	return b.String()
}
