// Posting back to the store from work that outlives the handler that started
// it.
//
// A verb that starts something long returns immediately and leaves a goroutine
// running. That goroutine has to report what happened - progress, then how it
// ended - and every one of them reached for context.Background() to do it,
// because the handler it came from had no context to inherit.
//
// Background() is wrong here in two ways. It carries nothing, so anything the
// caller's context held is lost; and it has no deadline, so a post to a store
// that has stopped answering strands the goroutine for the life of the process.
// A stopped store refuses rather than blocking, but "refuses" is a property of
// this store rather than of the call, and the call should not depend on it.
package session

import (
	"context"
	"time"
)

// postBackWindow bounds a post-back. It is generous: this is not a timeout that
// anything should reach, it is the difference between a bounded wait and none.
const postBackWindow = 10 * time.Second

// finishing returns the context for a post that says how a cancellable job
// ended.
//
// It inherits ctx, so whatever ctx carries goes with it, but not ctx's
// cancellation - a job being cancelled is precisely when the post saying so
// still has to go out. The one thing it adds is a bound.
//
// Use it for the terminal posts: job.done, the said reason, the result. For
// progress, pass the job's own ctx, because progress from a job that has been
// stopped is noise nobody asked for.
func finishing(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), postBackWindow)
}
