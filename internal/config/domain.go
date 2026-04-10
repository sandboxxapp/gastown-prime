package config

import (
	"os"
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
	// Content is the trimmed file content.
	Content string
}

// LoadDomainDocs loads all markdown files from the rig's domain directory.
// Resolution: <townRoot>/<rigName>/domain/**/*.md
//
// Files named README.md are skipped. Results are sorted by relative path
// for deterministic output. Returns nil if the domain directory doesn't
// exist or contains no docs.
func LoadDomainDocs(townRoot, rigName string) []DomainDoc {
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

		docs = append(docs, DomainDoc{
			RelPath:  rel,
			Title:    title,
			Category: category,
			Content:  trimmed,
		})

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
