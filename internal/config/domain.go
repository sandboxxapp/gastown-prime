package config

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DomainDoc represents a single domain documentation file.
type DomainDoc struct {
	// RelPath is the path relative to the domain/ directory (e.g. "auth/token.md").
	RelPath string
	// Title is derived from the filename (e.g. "Token" from "token.md").
	Title string
	// Category is the subdirectory name, empty for top-level files.
	Category string
	// Content is the trimmed file content. Populated by LoadDomainDocs only;
	// LoadDomainDocsTOC leaves this empty for memory efficiency.
	Content string
	// Summary is a short topic line extracted from frontmatter `summary:` or
	// falling back to the first non-heading paragraph. Truncated to ~120 chars.
	Summary string
	// LastTouched is the last-commit date in YYYY-MM-DD form, or empty if
	// the file is not in git or git is unavailable.
	LastTouched string
	// AbsPath is the absolute filesystem path. Internal use; not for templates.
	AbsPath string
}

// LoadDomainDocs loads all markdown files from the rig's domain directory
// with full content populated. Use LoadDomainDocsTOC for lightweight catalog
// queries that don't need the file body.
//
// Resolution: <townRoot>/<rigName>/domain/**/*.md
//
// Files named README.md are skipped. Results are sorted by relative path
// for deterministic output. Returns nil if the domain directory doesn't
// exist or contains no docs.
func LoadDomainDocs(townRoot, rigName string) []DomainDoc {
	return loadDomainDocs(townRoot, rigName, true)
}

// LoadDomainDocsTOC loads the lightweight catalog of domain docs without
// populating Content. Each entry includes RelPath, Title, Category, Summary,
// and LastTouched — enough to render a table of contents.
//
// This is what gt prime emits to polecats: a TOC they can read on demand
// instead of the entire content blob (typically ~360KB for a mature rig).
func LoadDomainDocsTOC(townRoot, rigName string) []DomainDoc {
	return loadDomainDocs(townRoot, rigName, false)
}

func loadDomainDocs(townRoot, rigName string, withContent bool) []DomainDoc {
	if rigName == "" {
		return nil
	}

	domainDir := filepath.Join(townRoot, rigName, "domain")
	info, err := os.Stat(domainDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var docs []DomainDoc

	err = filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if strings.EqualFold(info.Name(), "README.md") {
			return nil
		}

		content, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted config
		if err != nil {
			return nil // skip unreadable files
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			return nil
		}

		rel, _ := filepath.Rel(domainDir, path)
		category := filepath.Dir(rel)
		if category == "." {
			category = ""
		}

		// Title from filename: "referral-edge-cases.md" → "Referral Edge Cases"
		base := strings.TrimSuffix(info.Name(), ".md")
		title := TitleCaseHyphens(base)

		doc := DomainDoc{
			RelPath:     rel,
			Title:       title,
			Category:    category,
			Summary:     extractSummary(trimmed),
			LastTouched: gitLastTouched(path),
			AbsPath:     path,
		}
		if withContent {
			doc.Content = trimmed
		}

		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].RelPath < docs[j].RelPath
	})

	return docs
}

// extractSummary derives a one-line topic summary from a markdown document.
// Priority:
//  1. YAML frontmatter `summary: ...` field
//  2. First non-empty, non-heading line of the body
//
// Truncated to ~120 characters with an ellipsis if longer.
func extractSummary(content string) string {
	const maxLen = 120

	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Check for YAML frontmatter
	inFrontmatter := false
	frontmatterChecked := false
	var bodyLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if !frontmatterChecked {
			frontmatterChecked = true
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			bodyLines = append(bodyLines, line)
			continue
		}

		if inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "summary:") {
				summary := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "summary:"))
				summary = strings.Trim(summary, `"'`)
				return truncate(summary, maxLen)
			}
			continue
		}

		bodyLines = append(bodyLines, line)
	}

	inCodeFence := false
	for _, line := range bodyLines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue
		}
		return truncate(t, maxLen)
	}
	return ""
}

// truncate returns s clipped to at most n bytes, with an ellipsis ("…", 3
// bytes UTF-8) appended when truncation happens. The ellipsis counts toward
// the limit so the returned string never exceeds n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const ell = "…"
	cut := n - len(ell)
	if cut < 0 {
		cut = 0
	}
	// Walk back to a rune boundary so we never split a UTF-8 sequence.
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + ell
}

// gitLastTouched returns the last-commit date for the given file in
// YYYY-MM-DD form. Returns "" if git is unavailable or the file is not
// tracked. The lookup runs in the file's directory so it works for files
// inside nested repos / worktrees.
func gitLastTouched(absPath string) string {
	dir := filepath.Dir(absPath)
	cmd := exec.Command("git", "log", "-1", "--format=%ad", "--date=short", "--", filepath.Base(absPath)) //nolint:gosec // G204: args are filesystem-derived
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TitleCaseHyphens converts a hyphen-separated name to Title Case.
// "referral-edge-cases" → "Referral Edge Cases"
func TitleCaseHyphens(s string) string {
	words := strings.Split(s, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
