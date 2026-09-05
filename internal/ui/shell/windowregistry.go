// Which pop-out windows exist, and how another goroutine asks one forward.
//
// Extracted at the second kind of window rather than the first: node windows
// and firmware windows want exactly the same bookkeeping, and the part that is
// easy to get wrong - a second request raising rather than opening a duplicate,
// and a raise being a wish rather than a reach into somebody else's event loop
// - is not worth having two copies of.
package shell

import (
	"sort"
	"sync"
)

type WindowRegistry struct {
	mu   sync.Mutex
	open map[string]bool
	// raising is which windows have been asked to come forward: a wish
	// rather than an action, because a window belongs to its own event loop
	// and reaching into it from another goroutine is how a destroyed window
	// stays in Gio's queue.
	raising map[string]bool
	// closing is which have been asked to go away by something other than
	// their own bar - a panel put back into the main window, for one. A wish
	// like raising, and for the same reason.
	closing map[string]bool
}

// NewWindowRegistry returns a pointer, so that embedding one never copies the lock.
func NewWindowRegistry() *WindowRegistry {
	return &WindowRegistry{open: map[string]bool{}, raising: map[string]bool{},
		closing: map[string]bool{}}
}

// claim reports whether the caller should open this window.
//
// False means one is already out there, and it has been asked to come forward
// instead - which is also the escape hatch for a window dragged somewhere its
// bar cannot be reached from, where doing nothing would read as a dead menu
// entry.
func (w *WindowRegistry) Claim(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.open[key] {
		w.raising[key] = true
		return false
	}
	w.open[key] = true
	return true
}

// release forgets a window that has closed.
func (w *WindowRegistry) Release(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.open, key)
	delete(w.raising, key)
	delete(w.closing, key)
}

// wantsRaise reports and clears the wish, on the window's own goroutine.
func (w *WindowRegistry) WantsRaise(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising[key] {
		delete(w.raising, key)
		return true
	}
	return false
}

// Has reports whether a window is out there. Read from the interface, which
// draws "put it back" against a panel that is popped out and "pop it out"
// against one that is not.
func (w *WindowRegistry) Has(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.open[key]
}

// AskClose wishes a window closed from outside its own event loop.
func (w *WindowRegistry) AskClose(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.open[key] {
		w.closing[key] = true
	}
}

// WantsClose reports and clears that wish, on the window's own goroutine.
func (w *WindowRegistry) WantsClose(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing[key] {
		delete(w.closing, key)
		return true
	}
	return false
}

// Keys is every window that is out there, sorted, for an interface that lists
// them.
func (w *WindowRegistry) Keys() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.open))
	for k := range w.open {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
