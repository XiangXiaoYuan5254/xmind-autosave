package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigurationKeepsDefaultsForMissingKeys(t *testing.T) {
	configuration, err := ParseConfiguration([]byte(`{"saveDelayMilliseconds": 800}`))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SaveDelay() != 800*time.Millisecond {
		t.Fatalf("save delay = %v", configuration.SaveDelay())
	}
	if configuration.PollInterval() != 200*time.Millisecond || configuration.RetryInterval() != 2*time.Second {
		t.Fatalf("unexpected defaults: %+v", configuration)
	}
	if len(configuration.ProcessNames) == 0 || len(configuration.DirtyIndicators) == 0 {
		t.Fatalf("defaults lost: %+v", configuration)
	}
}

func TestParseConfigurationClampsIntervals(t *testing.T) {
	configuration, err := ParseConfiguration([]byte(`{"pollIntervalMilliseconds": 1, "retryMilliseconds": 0, "saveDelayMilliseconds": -5}`))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.PollInterval() != 100*time.Millisecond ||
		configuration.RetryInterval() != 500*time.Millisecond ||
		configuration.SaveDelay() != 0 {
		t.Fatalf("intervals not clamped: %+v", configuration)
	}
}

func TestSourceFilePath(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{
			"file:///C:/Program%20Files/Xmind/resources/app.asar/index.html?source=file%3A%2F%2F%2FC%3A%2FUsers%2Fme%2FDocuments%2F%E6%B5%8B%E8%AF%95.xmind",
			`C:\Users\me\Documents\测试.xmind`,
		},
		{
			"app://xmind/editor.html?windowId=3&source=file:///D:/Notes/a%20b.xmind",
			`D:\Notes\a b.xmind`,
		},
		{
			"file:///C:/xmind/index.html#/editor?source=file%3A%2F%2F%2FE%3A%2Fmaps%2Fplan.xmind",
			`E:\maps\plan.xmind`,
		},
		{
			"file:///C:/xmind/index.html?source=file%3A%2F%2Fnas%2Fshare%2Fteam.xmind",
			`\\nas\share\team.xmind`,
		},
		{
			"file:///C:/xmind/index.html?source=C%3A%5Cdocs%5Cplain.xmind",
			`C:\docs\plain.xmind`,
		},
		{
			"file:///Applications/Xmind.app/index.html?source=file%3A%2F%2F%2FUsers%2Fme%2Fmap.xmind",
			"/Users/me/map.xmind",
		},
	}
	for _, testCase := range cases {
		got, ok := SourceFilePath(testCase.url)
		if !ok || got != testCase.want {
			t.Errorf("SourceFilePath(%q) = %q, %v; want %q", testCase.url, got, ok, testCase.want)
		}
	}
}

func TestSourceFilePathRejectsNonLocalDocuments(t *testing.T) {
	for _, rawURL := range []string{
		"file:///C:/xmind/index.html",
		"file:///C:/xmind/index.html?source=https%3A%2F%2Fxmind.ai%2Fshare%2Fabc",
		"file:///C:/xmind/index.html?source=untitled",
		"not a url %%%",
	} {
		if got, ok := SourceFilePath(rawURL); ok {
			t.Errorf("SourceFilePath(%q) = %q; want no match", rawURL, got)
		}
	}
}

func TestDocumentStem(t *testing.T) {
	for path, want := range map[string]string{
		`C:\docs\计划.xmind`:    "计划",
		"/Users/me/a.b.xmind": "a.b",
		`plain`:               "plain",
		`C:\docs\.hidden`:     ".hidden",
	} {
		if got := DocumentStem(path); got != want {
			t.Errorf("DocumentStem(%q) = %q; want %q", path, got, want)
		}
	}
}

func TestTitleIndicatesDirtyIgnoresDocumentName(t *testing.T) {
	indicators := []string{"已编辑", "Edited", "*"}
	if !TitleIndicatesDirty("*计划 - Xmind", indicators, "计划") {
		t.Error("asterisk marker not detected")
	}
	if !TitleIndicatesDirty("计划 — 已编辑", indicators, "计划") {
		t.Error("已编辑 marker not detected")
	}
	if TitleIndicatesDirty("Edited notes - Xmind", indicators, "Edited notes") {
		t.Error("document name must not count as a marker")
	}
	if !TitleIndicatesDirty("Edited notes — edited", indicators, "Edited notes") {
		t.Error("marker after the document name not detected")
	}
}

func TestNormalizedTitle(t *testing.T) {
	indicators := []string{"已编辑", "*"}
	if got := NormalizedTitle(" *计划 - Xmind ", indicators); got != "计划 - Xmind" {
		t.Errorf("NormalizedTitle = %q", got)
	}
}

func TestMatchDocumentName(t *testing.T) {
	names := []string{"计划", "计划 2026", "in", "Notes"}
	cases := map[string]int{
		"计划 2026 - Xmind":      1,
		"*计划 - Xmind":          0,
		"计划书 - Xmind":          -1,
		"notes.xmind":          3,
		"Xmind":                -1,
		"Weekly Notes - Xmind": 3,
	}
	for title, want := range cases {
		if got := MatchDocumentName(title, names); got != want {
			t.Errorf("MatchDocumentName(%q) = %d; want %d", title, got, want)
		}
	}
}

func TestPerFileStoreFollowsIdentityAndPath(t *testing.T) {
	directory := t.TempDir()
	storage := filepath.Join(directory, "sub", "preferences.json")
	identities := map[string]string{`C:\a.xmind`: "file:1:100"}
	keyFor := func(path string) string {
		if key, ok := identities[strings.ToLower(path)]; ok {
			return key
		}
		if key, ok := identities[path]; ok {
			return key
		}
		return PathKeyPrefix + strings.ToLower(path)
	}

	store := NewPerFileStore(storage, keyFor, nil)
	if store.IsEnabled(`C:\a.xmind`) {
		t.Fatal("new documents must start disabled")
	}
	if err := store.SetEnabled(true, `C:\a.xmind`); err != nil {
		t.Fatal(err)
	}

	// Renamed on the same volume: same identity, new path.
	delete(identities, `C:\a.xmind`)
	identities[`C:\renamed.xmind`] = "file:1:100"
	if !NewPerFileStore(storage, keyFor, nil).IsEnabled(`C:\renamed.xmind`) {
		t.Fatal("setting lost after rename")
	}

	// Replaced on save: new identity, same path.
	identities[`C:\renamed.xmind`] = "file:1:200"
	reloaded := NewPerFileStore(storage, keyFor, nil)
	if !reloaded.IsEnabled(`c:/RENAMED.xmind`) {
		t.Fatal("setting lost after the file was replaced")
	}
	if !NewPerFileStore(storage, keyFor, nil).IsEnabled(`C:\renamed.xmind`) {
		t.Fatal("migrated entry was not persisted")
	}

	if err := reloaded.SetEnabled(false, `C:\renamed.xmind`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(storage)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "lastKnownPath") != 1 {
		t.Fatalf("stale entries left behind:\n%s", data)
	}
}

func TestPerFileStoreInitiallyEnabledPaths(t *testing.T) {
	keyFor := func(path string) string { return PathKeyPrefix + strings.ToLower(path) }
	store := NewPerFileStore(filepath.Join(t.TempDir(), "p.json"), keyFor, []string{`C:\Docs\A.xmind`})
	if !store.IsEnabled(`c:\docs\a.xmind`) {
		t.Fatal("configured file should start enabled")
	}
	if err := store.SetEnabled(false, `C:\Docs\A.xmind`); err != nil {
		t.Fatal(err)
	}
	if store.IsEnabled(`C:\Docs\A.xmind`) {
		t.Fatal("stored choice must win over the configuration")
	}
}

// schedulerHarness drives a Scheduler through a timeline.
type schedulerHarness struct {
	t         *testing.T
	scheduler *Scheduler
	start     time.Time
	now       time.Time
	base      Observation
}

func newSchedulerHarness(t *testing.T) *schedulerHarness {
	start := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	return &schedulerHarness{
		t:         t,
		scheduler: NewScheduler(1200*time.Millisecond, 2*time.Second),
		start:     start,
		now:       start,
		base: Observation{
			Path:    `C:\a.xmind`,
			Enabled: true,
			Focused: true,
			ModTime: start.Add(-time.Hour),
		},
	}
}

func (h *schedulerHarness) at(milliseconds int) *schedulerHarness {
	h.now = h.start.Add(time.Duration(milliseconds) * time.Millisecond)
	return h
}

func (h *schedulerHarness) edit() {
	h.scheduler.NoteEdit(h.base.Path, h.now)
}

func (h *schedulerHarness) poll(adjust ...func(*Observation)) Decision {
	observation := h.base
	observation.Now = h.now
	for _, change := range adjust {
		change(&observation)
	}
	return h.scheduler.Evaluate(observation)
}

func (h *schedulerHarness) expect(decision Decision, save bool, status SaveStatus) {
	h.t.Helper()
	if decision.Save != save || decision.Status != status {
		h.t.Fatalf("at %v: got save=%v status=%v; want save=%v status=%v",
			h.now.Sub(h.start), decision.Save, decision.Status, save, status)
	}
}

func TestSchedulerWaitsForQuietBeforeSaving(t *testing.T) {
	h := newSchedulerHarness(t)
	h.expect(h.at(0).poll(), false, StatusSaved)

	h.at(100).edit()
	h.expect(h.at(200).poll(), false, StatusEditing)
	h.at(1000).edit() // typing again restarts the timer
	h.expect(h.at(2000).poll(), false, StatusEditing)
	h.expect(h.at(2200).poll(), true, StatusSaving)
	h.expect(h.at(2400).poll(), false, StatusSaving)

	h.base.ModTime = h.start.Add(2500 * time.Millisecond)
	decision := h.at(2600).poll()
	h.expect(decision, false, StatusAutoSaved)
	if !decision.LastSaved.Equal(h.now) {
		t.Fatalf("LastSaved = %v", decision.LastSaved)
	}
}

func TestSchedulerSettlesWhenNothingWasWritten(t *testing.T) {
	h := newSchedulerHarness(t)
	h.at(0).edit()
	h.expect(h.at(1300).poll(), true, StatusSaving)
	h.expect(h.at(3000).poll(), false, StatusSaving)
	h.expect(h.at(6400).poll(), false, StatusSaved)
}

func TestSchedulerWaitsForFocusAndIdleInput(t *testing.T) {
	h := newSchedulerHarness(t)
	h.at(0).edit()
	unfocused := func(o *Observation) { o.Focused = false }
	h.expect(h.at(1500).poll(unfocused), false, StatusWaitingForFocus)
	held := func(o *Observation) { o.InputHeld = true }
	h.expect(h.at(1700).poll(held), false, StatusEditing)
	composing := func(o *Observation) { o.Composing = true }
	h.expect(h.at(1900).poll(composing), false, StatusEditing)
	h.expect(h.at(2100).poll(), true, StatusSaving)
}

func TestSchedulerForceSavesImmediately(t *testing.T) {
	h := newSchedulerHarness(t)
	force := func(o *Observation) { o.Force = true }
	h.expect(h.at(0).poll(force), true, StatusSaving)

	h.at(100).edit()
	h.expect(h.at(200).poll(force, func(o *Observation) { o.Composing = true }), true, StatusSaving)
}

func TestSchedulerFollowsTitleMarkersOnceSeen(t *testing.T) {
	h := newSchedulerHarness(t)
	dirty := func(o *Observation) { o.TitleDirty = true }

	h.expect(h.at(0).poll(dirty), false, StatusEditing)
	h.expect(h.at(1300).poll(dirty), true, StatusSaving)
	// Still marked dirty right after the attempt: wait for the retry interval.
	h.expect(h.at(1500).poll(dirty), false, StatusSaving)
	h.expect(h.at(3400).poll(dirty), true, StatusSaving)
	h.expect(h.at(3600).poll(), false, StatusAutoSaved)

	// From now on input alone no longer counts as a change…
	h.at(4000).edit()
	h.expect(h.at(5500).poll(), false, StatusAutoSaved)
	// …but it still restarts the timer while the title says unsaved.
	h.expect(h.at(6000).poll(dirty), false, StatusEditing)
	h.at(6500).edit()
	h.expect(h.at(7300).poll(dirty), false, StatusEditing)
	h.expect(h.at(7800).poll(dirty), true, StatusSaving)
}

func TestSchedulerForgetsDisabledDocuments(t *testing.T) {
	h := newSchedulerHarness(t)
	h.at(0).edit()
	h.expect(h.at(100).poll(func(o *Observation) { o.Enabled = false }), false, StatusDisabled)
	h.expect(h.at(2000).poll(), false, StatusSaved)
}
