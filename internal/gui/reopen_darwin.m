// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

#import <Cocoa/Cocoa.h>
#include "_cgo_export.h"

@interface NebulaReopenHandler : NSObject
- (void)handleReopen:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply;
@end

@implementation NebulaReopenHandler
- (void)handleReopen:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
	nebulaReopened();
}
@end

// nebulaWatchReopen routes the reopen Apple Event to nebulaReopened.
void nebulaWatchReopen(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		static NebulaReopenHandler *handler; // kept for the life of the app
		if (handler == nil) {
			handler = [[NebulaReopenHandler alloc] init];
		}
		[[NSAppleEventManager sharedAppleEventManager]
			setEventHandler:handler
			    andSelector:@selector(handleReopen:withReplyEvent:)
			  forEventClass:kCoreEventClass
			     andEventID:kAEReopenApplication];
	});
}
