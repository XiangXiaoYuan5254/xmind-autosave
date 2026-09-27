package core

import "time"

type SaveStatus int

const (
	StatusDisabled SaveStatus = iota
	StatusSaved
	StatusAutoSaved
	StatusEditing
	StatusSaving
	StatusWaitingForFocus
)

// Observation is what the app sees of the current document on one poll.
type Observation struct {
	Now     time.Time
	Path    string
	Enabled bool
	// TitleDirty is true while the window title carries an unsaved marker.
	TitleDirty bool
	// ModTime of the document on disk; zero when unknown.
	ModTime time.Time
	// Focused is true when the document window is in the foreground, so a
	// synthesized Ctrl+S would reach it.
	Focused bool
	// InputHeld is true while a modifier key or mouse button is down.
	InputHeld bool
	// Composing is true while an input method may still be composing text.
	Composing bool
	// Force asks for a save as soon as possible ("立即检查并保存").
	Force bool
}

type Decision struct {
	Save      bool
	Status    SaveStatus
	LastSaved time.Time
}

// confirmationWindow is how long a save attempt waits for the file on disk
// (or the title marker) to confirm it.
const confirmationWindow = 5 * time.Second

// Scheduler decides when to send Ctrl+S. Every edit restarts the quiet timer;
// the save goes out once the document has been quiet for the save delay.
//
// XMind for macOS marks unsaved documents in the title bar, which is what the
// macOS version waits for. If the Windows title carries such a marker too, the
// scheduler uses it the same way. Until one has been seen, any keyboard or
// mouse edit in the document counts as a change; Ctrl+S on an unchanged local
// document does nothing, so the cost of a false positive is nil.
type Scheduler struct {
	delay            time.Duration
	retry            time.Duration
	titleMarkersSeen bool
	states           map[string]*saveState
}

type saveState struct {
	pendingEdit         bool
	lastEdit            time.Time
	titleDirtySince     time.Time
	lastAttempt         time.Time
	awaitingSave        bool
	modTimeAtAttempt    time.Time
	titleDirtyAtAttempt bool
	lastSaved           time.Time
}

func NewScheduler(delay, retry time.Duration) *Scheduler {
	return &Scheduler{delay: delay, retry: retry, states: map[string]*saveState{}}
}

// NoteEdit records input in the document that may have changed it.
func (s *Scheduler) NoteEdit(path string, at time.Time) {
	state := s.state(path)
	state.pendingEdit = true
	if at.After(state.lastEdit) {
		state.lastEdit = at
	}
}

func (s *Scheduler) Forget(path string) {
	delete(s.states, path)
}

func (s *Scheduler) Evaluate(o Observation) Decision {
	if !o.Enabled {
		delete(s.states, o.Path)
		return Decision{Status: StatusDisabled}
	}
	state := s.state(o.Path)
	state.confirmSave(o)

	if o.TitleDirty {
		s.titleMarkersSeen = true
		if state.titleDirtySince.IsZero() {
			state.titleDirtySince = o.Now
		}
	} else {
		state.titleDirtySince = time.Time{}
	}
	if o.Force {
		state.pendingEdit = true
	}

	dirty := o.TitleDirty || (state.pendingEdit && (o.Force || !s.titleMarkersSeen))
	if !dirty {
		state.pendingEdit = false
		return state.decision(state.restingStatus())
	}
	if !o.Focused {
		return state.decision(StatusWaitingForFocus)
	}

	lastEdit := state.lastEdit
	if state.titleDirtySince.After(lastEdit) {
		lastEdit = state.titleDirtySince
	}
	quiet := o.Now.Sub(lastEdit) >= s.delay
	mayRetry := state.lastAttempt.IsZero() ||
		lastEdit.After(state.lastAttempt) ||
		o.Now.Sub(state.lastAttempt) >= s.retry
	due := o.Force || (quiet && mayRetry && !o.Composing)

	if !due || o.InputHeld {
		if state.awaitingSave {
			return state.decision(StatusSaving)
		}
		return state.decision(StatusEditing)
	}

	state.pendingEdit = false
	state.lastAttempt = o.Now
	state.awaitingSave = true
	state.modTimeAtAttempt = o.ModTime
	state.titleDirtyAtAttempt = o.TitleDirty
	decision := state.decision(StatusSaving)
	decision.Save = true
	return decision
}

func (s *Scheduler) state(path string) *saveState {
	state := s.states[path]
	if state == nil {
		state = &saveState{}
		s.states[path] = state
	}
	return state
}

func (state *saveState) confirmSave(o Observation) {
	if !state.awaitingSave {
		return
	}
	switch {
	case !state.modTimeAtAttempt.IsZero() && o.ModTime.After(state.modTimeAtAttempt),
		state.titleDirtyAtAttempt && !o.TitleDirty:
		state.lastSaved = o.Now
		state.awaitingSave = false
	case o.Now.Sub(state.lastAttempt) >= confirmationWindow:
		// Nothing changed on disk: the document was already saved.
		state.awaitingSave = false
	}
}

func (state *saveState) restingStatus() SaveStatus {
	switch {
	case state.awaitingSave:
		return StatusSaving
	case !state.lastSaved.IsZero():
		return StatusAutoSaved
	default:
		return StatusSaved
	}
}

func (state *saveState) decision(status SaveStatus) Decision {
	return Decision{Status: status, LastSaved: state.lastSaved}
}
