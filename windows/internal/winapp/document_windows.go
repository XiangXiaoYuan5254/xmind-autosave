package winapp

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

// Finding the file behind an XMind window.
//
// 1. Accessibility (preferred, exact): XMind's editor page is loaded with the
//    document in its URL (…?source=file:///…/a.xmind), the same URL the macOS
//    version reads. Chromium exposes a document's URL as its MSAA value.
// 2. Recent items (fallback): Windows keeps a shortcut for every document
//    opened through Explorer or a file dialog in the Recent folder. The
//    shortcut whose name appears in the window title identifies the file.

const (
	renderWidgetClass     = "Chrome_RenderWidgetHostHWND"
	accessibleNodeBudget  = 600
	accessibleTimeBudget  = 700 * time.Millisecond
	accessibleChildLimit  = 200
	accessibleDepthLimit  = 40
	methodAccessibility   = "accessibility"
	methodRecentShortcuts = "recent"
)

type accessibleVisitor func(node *comObject, depth int, role int32) (stop bool)

// walkAccessible visits the tree breadth-first within the node and time
// budgets. It releases every node it created; root stays owned by the caller.
func walkAccessible(root *comObject, nodeBudget int, deadline time.Time, visit accessibleVisitor) int {
	type queued struct {
		node  *comObject
		depth int
		owned bool
	}
	queue := []queued{{node: root}}
	defer func() {
		for _, item := range queue {
			if item.owned {
				item.node.release()
			}
		}
	}()

	visited := 0
	for len(queue) > 0 && visited < nodeBudget && time.Now().Before(deadline) {
		item := queue[0]
		queue = queue[1:]
		visited++

		stop := visit(item.node, item.depth, item.node.accessibleRole())
		if !stop && item.depth < accessibleDepthLimit {
			for _, child := range item.node.accessibleChildren(accessibleChildLimit) {
				queue = append(queue, queued{node: child, depth: item.depth + 1, owned: true})
			}
		}
		if item.owned {
			item.node.release()
		}
		if stop {
			break
		}
	}
	return visited
}

// accessibleRoots returns the accessibility roots of an XMind window: the
// render widget windows first (their root is the web document itself), then
// the top-level window.
func accessibleRoots(hwnd uintptr) []*comObject {
	var roots []*comObject
	for _, child := range enumerateWindows(hwnd) {
		if windowClass(child) == renderWidgetClass {
			if root, err := accessibleFromWindow(child); err == nil {
				roots = append(roots, root)
			}
		}
	}
	if root, err := accessibleFromWindow(hwnd); err == nil {
		roots = append(roots, root)
	}
	for _, root := range roots {
		root.requestWebAccessibility()
	}
	return roots
}

func documentPathFromAccessibility(hwnd uintptr) (string, error) {
	roots := accessibleRoots(hwnd)
	defer func() {
		for _, root := range roots {
			root.release()
		}
	}()
	if len(roots) == 0 {
		return "", errors.New("no accessibility object for the window")
	}

	deadline := time.Now().Add(accessibleTimeBudget)
	budget := accessibleNodeBudget
	for _, root := range roots {
		var found string
		budget -= walkAccessible(root, budget, deadline, func(node *comObject, _ int, role int32) bool {
			if role != roleDocument {
				return false
			}
			if path, ok := core.SourceFilePath(node.accessibleString(accValue)); ok {
				found = path
				return true
			}
			return false
		})
		if found != "" {
			return filepath.Clean(found), nil
		}
		if budget <= 0 || time.Now().After(deadline) {
			break
		}
	}
	return "", errors.New("no document URL with a local source")
}

type recentDocument struct {
	name     string
	shortcut string
	modified time.Time
}

func recentFolder() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Recent`)
}

// recentDocuments lists the .xmind shortcuts in the Recent folder, newest first.
func recentDocuments() []recentDocument {
	entries, err := os.ReadDir(recentFolder())
	if err != nil {
		return nil
	}
	const suffix = ".xmind.lnk"
	var documents []recentDocument
	for _, entry := range entries {
		name := entry.Name()
		if len(name) <= len(suffix) || !strings.EqualFold(name[len(name)-len(suffix):], suffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		documents = append(documents, recentDocument{
			name:     name[:len(name)-len(suffix)],
			shortcut: filepath.Join(recentFolder(), name),
			modified: info.ModTime(),
		})
	}
	sort.SliceStable(documents, func(i, j int) bool {
		return documents[i].modified.After(documents[j].modified)
	})
	return documents
}

func documentPathFromRecent(title string) (string, error) {
	documents := recentDocuments()
	names := make([]string, len(documents))
	for index, document := range documents {
		names[index] = document.name
	}
	index := core.MatchDocumentName(title, names)
	if index < 0 {
		return "", errors.New("no recent document matches the window title")
	}
	target, err := shortcutTarget(documents[index].shortcut)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		return "", errors.New("recent document no longer exists")
	}
	return filepath.Clean(target), nil
}
