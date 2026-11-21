// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include "_cgo_export.h"

// current is the open panel, so nebulaPickCancel can close it. Only the
// main thread touches it.
static NSSavePanel *current;

// nebulaPickFile shows an open panel, or with save set a save panel, and
// calls nebulaPicked with the answer. window is the NSWindow to show it on
// as a sheet, or 0. extensions is a newline-separated list without dots;
// empty offers every file. The strings are copied before it returns.
void nebulaPickFile(uintptr_t handle, uintptr_t window, int save, const char *title,
	const char *fileName, const char *startDir, const char *extensions) {
	NSString *t = [NSString stringWithUTF8String:title];
	NSString *name = [NSString stringWithUTF8String:fileName];
	NSString *dir = [NSString stringWithUTF8String:startDir];
	NSString *exts = [NSString stringWithUTF8String:extensions];
	dispatch_async(dispatch_get_main_queue(), ^{
		NSSavePanel *panel;
		if (save) {
			panel = [NSSavePanel savePanel];
			panel.canCreateDirectories = YES;
			if (name.length > 0) {
				panel.nameFieldStringValue = name;
			}
		} else {
			NSOpenPanel *open = [NSOpenPanel openPanel];
			open.canChooseFiles = YES;
			open.canChooseDirectories = NO;
			open.allowsMultipleSelection = NO;
			panel = open;
		}
		if (t.length > 0) {
			panel.title = t;
			panel.message = t; // a sheet shows the message, not the title
		}
		if (dir.length > 0) {
			panel.directoryURL = [NSURL fileURLWithPath:dir isDirectory:YES];
		}
		if (exts.length > 0) {
			NSMutableArray<UTType *> *types = [NSMutableArray array];
			for (NSString *e in [exts componentsSeparatedByString:@"\n"]) {
				UTType *u = [UTType typeWithFilenameExtension:e];
				if (u != nil) {
					[types addObject:u];
				}
			}
			if (types.count > 0) {
				panel.allowedContentTypes = types;
			}
		}

		current = [panel retain];
		void (^finish)(NSModalResponse) = ^(NSModalResponse r) {
			NSString *path = nil;
			if (r == NSModalResponseOK) {
				path = panel.URL.path;
			}
			if (current == panel) {
				current = nil;
			}
			nebulaPicked(handle, path ? (char *)path.fileSystemRepresentation : NULL);
			[panel release];
		};

		NSWindow *w = (NSWindow *)window;
		if (w != nil && w.visible) {
			[panel beginSheetModalForWindow:w completionHandler:finish];
		} else {
			[NSApp activateIgnoringOtherApps:YES];
			[panel beginWithCompletionHandler:finish];
		}
	});
}

// nebulaPickCancel closes the open panel, if any, as if the user had
// pressed Cancel.
void nebulaPickCancel(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[current cancel:nil];
	});
}
