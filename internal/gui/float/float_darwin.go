package float

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// Floating, not modal or status: above normal windows, below panels the
// system itself owns, and it stops floating when the app is in the
// background - which is what "keep my tool windows in front" means.
static void meshbenchFloat(uintptr_t view) {
	NSView *v = (__bridge NSView *)(void *)view;
	dispatch_async(dispatch_get_main_queue(), ^{
		[[v window] setLevel:NSFloatingWindowLevel];
		[[v window] setHidesOnDeactivate:NO];
	});
}
*/
import "C"

import "gioui.org/app"

func keep(e app.ViewEvent) bool {
	v, ok := e.(app.AppKitViewEvent)
	if !ok || v.View == 0 {
		return false
	}
	C.meshbenchFloat(C.uintptr_t(v.View))
	return true
}
