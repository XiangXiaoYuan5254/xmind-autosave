package core

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SourceFilePath extracts the local document path from the URL of XMind's
// editor page, e.g. "…/index.html?source=file%3A%2F%2F%2FC%3A%2Fdocs%2Fa.xmind".
// It mirrors XMindDocumentInspector.sourceFilePath on macOS, and also accepts
// a query string inside the fragment ("#/editor?source=…").
func SourceFilePath(editorURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(editorURL))
	if err != nil {
		return "", false
	}

	queries := []url.Values{parsed.Query()}
	if index := strings.IndexByte(parsed.Fragment, '?'); index >= 0 {
		if values, err := url.ParseQuery(parsed.Fragment[index+1:]); err == nil {
			queries = append(queries, values)
		}
	}

	for _, values := range queries {
		for _, source := range values["source"] {
			if path, ok := localPathFromSource(source); ok {
				return path, true
			}
		}
	}
	return "", false
}

func localPathFromSource(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if len(source) >= 5 && strings.EqualFold(source[:5], "file:") {
		return pathFromFileURL(source)
	}
	if isWindowsAbsolutePath(source) {
		return toBackslashes(source), true
	}
	return "", false
}

func pathFromFileURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return "", false
	}

	path := parsed.Path
	if parsed.Opaque != "" {
		// "file:C:/docs/a.xmind" has no authority, so the path ends up opaque.
		unescaped, err := url.PathUnescape(parsed.Opaque)
		if err != nil {
			return "", false
		}
		path = unescaped
	}

	if host := parsed.Host; host != "" && !strings.EqualFold(host, "localhost") {
		// file://server/share/a.xmind is a UNC path.
		return `\\` + host + toBackslashes(path), true
	}
	if len(path) >= 3 && path[0] == '/' && hasDrivePrefix(path[1:]) {
		return toBackslashes(path[1:]), true
	}
	if hasDrivePrefix(path) {
		return toBackslashes(path), true
	}
	if strings.HasPrefix(path, "/") {
		// POSIX path, as produced by XMind for macOS.
		return path, true
	}
	return "", false
}

func hasDrivePrefix(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	letter := path[0] | 0x20
	if letter < 'a' || letter > 'z' {
		return false
	}
	return len(path) == 2 || path[2] == '/' || path[2] == '\\'
}

func isWindowsAbsolutePath(path string) bool {
	return (hasDrivePrefix(path) && len(path) > 2) || strings.HasPrefix(path, `\\`)
}

func toBackslashes(path string) string {
	return strings.ReplaceAll(path, "/", `\`)
}

// DocumentStem returns the file name without directory and extension.
func DocumentStem(path string) string {
	name := path[strings.LastIndexAny(path, `/\`)+1:]
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		name = name[:dot]
	}
	return name
}

// TitleIndicatesDirty reports whether the window title carries one of the
// "unsaved" markers. The document's own name is ignored first, so a file
// called "Edited notes.xmind" does not look permanently dirty.
func TitleIndicatesDirty(title string, indicators []string, documentName string) bool {
	remaining := title
	if documentName != "" {
		remaining = removeFold(remaining, documentName)
	}
	for _, indicator := range indicators {
		if indicator != "" && indexFold(remaining, indicator) >= 0 {
			return true
		}
	}
	return false
}

// NormalizedTitle strips the unsaved markers so that a title flipping between
// "a" and "*a" while editing still identifies the same document.
func NormalizedTitle(title string, indicators []string) string {
	for _, indicator := range indicators {
		if indicator != "" {
			title = removeFold(title, indicator)
		}
	}
	return strings.TrimSpace(title)
}

// MatchDocumentName picks the name that appears in the window title as a
// whole word. The longest match wins; ties go to the earliest entry, so
// callers should order names by preference (e.g. most recently used first).
// It returns -1 when nothing matches.
func MatchDocumentName(title string, names []string) int {
	best := -1
	for index, name := range names {
		if name == "" || !containsWordFold(title, name) {
			continue
		}
		if best < 0 || utf8.RuneCountInString(name) > utf8.RuneCountInString(names[best]) {
			best = index
		}
	}
	return best
}

func containsWordFold(s, word string) bool {
	for offset := 0; offset < len(s); {
		index := indexFold(s[offset:], word)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(word)
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if (start == 0 || isWordBoundary(before)) && (end == len(s) || isWordBoundary(after)) {
			return true
		}
		_, size := utf8.DecodeRuneInString(s[start:])
		offset = start + size
	}
	return false
}

func isWordBoundary(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func indexFold(s, substr string) int {
	if substr == "" {
		return 0
	}
	for index := 0; index+len(substr) <= len(s); index++ {
		if utf8.RuneStart(s[index]) && strings.EqualFold(s[index:index+len(substr)], substr) {
			return index
		}
	}
	return -1
}

func removeFold(s, substr string) string {
	if substr == "" {
		return s
	}
	var builder strings.Builder
	for {
		index := indexFold(s, substr)
		if index < 0 {
			builder.WriteString(s)
			return builder.String()
		}
		builder.WriteString(s[:index])
		s = s[index+len(substr):]
	}
}
