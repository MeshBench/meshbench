// Which pop-out windows exist, and how another goroutine asks one forward.
//
// Extracted at the second kind of window rather than the first: node windows
// and firmware windows want exactly the same bookkeeping, and the part that is
// easy to get wrong - a second request raising rather than opening a duplicate,
// and a raise being a wish rather than a reach into somebody else's event loop
// - is not worth having two copies of.
package workbench

import "sync"

type windowSet struct {
	mu   sync.Mutex
	open map[string]bool
	// raising is which windows have been asked to come forward: a wish
	// rather than an action, because a window belongs to its own event loop
	// and reaching into it from another goroutine is how a destroyed window
	// stays in Gio's queue.
	raising map[string]bool
}

// newWindowSet returns a pointer, so that embedding one never copies the lock.
func newWindowSet() *windowSet {
	return &windowSet{open: map[string]bool{}, raising: map[string]bool{}}
}

// claim reports whether the caller should open this window.
//
// False means one is already out there, and it has been asked to come forward
// instead - which is also the escape hatch for a window dragged somewhere its
// bar cannot be reached from, where doing nothing would read as a dead menu
// entry.
func (w *windowSet) claim(key string) bool {
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
func (w *windowSet) release(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.open, key)
	delete(w.raising, key)
}

// wantsRaise reports and clears the wish, on the window's own goroutine.
func (w *windowSet) wantsRaise(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising[key] {
		delete(w.raising, key)
		return true
	}
	return false
}
