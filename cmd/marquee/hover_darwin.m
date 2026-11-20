// Hover preview: resting the pointer on the menu-bar icon shows a live mini view of the panel
// beneath it, so Marquee can be watched with no panel connected. systray owns the status item,
// so this finds the status bar window that belongs to this app and watches it with a tracking
// area. Frames come from the engine through mqPreviewFrame (hover_darwin.go).

#import <Cocoa/Cocoa.h>

extern void *mqPreviewFrame(int *n);
extern char *mqPreviewTitle(void);

static const CGFloat kW = 600, kH = 144, kPad = 6, kCap = 22;

@interface MQHover : NSObject
@property(strong) NSPanel *panel;
@property(strong) NSImageView *view;
@property(strong) NSTextField *caption;
@property(strong) NSWindow *anchor;
@property(strong) NSTimer *timer;
@property BOOL inside;
@property BOOL menuOpen;
@end

@implementation MQHover

- (void)build {
	NSRect r = NSMakeRect(0, 0, kW + 2 * kPad, kH + 2 * kPad + kCap);
	self.panel = [[NSPanel alloc] initWithContentRect:r
	                                        styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
	                                          backing:NSBackingStoreBuffered
	                                            defer:YES];
	self.panel.level = NSStatusWindowLevel;
	self.panel.opaque = NO;
	self.panel.backgroundColor = NSColor.clearColor;
	self.panel.hasShadow = NO;
	self.panel.ignoresMouseEvents = YES;
	self.panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary | NSWindowCollectionBehaviorFullScreenAuxiliary;
	NSView *box = [[NSView alloc] initWithFrame:r];
	box.wantsLayer = YES;
	box.layer.backgroundColor = [NSColor colorWithSRGBRed:12 / 255.0 green:13 / 255.0 blue:11 / 255.0 alpha:1].CGColor;
	box.layer.cornerRadius = 4;
	box.layer.borderWidth = 1;
	box.layer.borderColor = [NSColor colorWithWhite:1 alpha:0.14].CGColor;
	self.panel.contentView = box;
	self.view = [[NSImageView alloc] initWithFrame:NSMakeRect(kPad, kPad + kCap, kW, kH)];
	self.view.imageScaling = NSImageScaleAxesIndependently;
	[box addSubview:self.view];
	self.caption = [NSTextField labelWithString:@""];
	self.caption.frame = NSMakeRect(kPad + 2, 4, kW - 4, kCap - 6);
	self.caption.font = [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightMedium];
	self.caption.textColor = [NSColor colorWithSRGBRed:230 / 255.0 green:226 / 255.0 blue:214 / 255.0 alpha:0.8];
	self.caption.lineBreakMode = NSLineBreakByTruncatingTail;
	[box addSubview:self.caption];
}

- (BOOL)attach {
	for (NSWindow *w in NSApp.windows) {
		if ([NSStringFromClass(w.class) containsString:@"StatusBarWindow"] && w.contentView) {
			self.anchor = w;
			NSTrackingArea *t = [[NSTrackingArea alloc]
			    initWithRect:NSZeroRect
			         options:NSTrackingMouseEnteredAndExited | NSTrackingActiveAlways | NSTrackingInVisibleRect
			           owner:self
			        userInfo:nil];
			[w.contentView addTrackingArea:t];
			return YES;
		}
	}
	return NO;
}

- (void)mouseEntered:(NSEvent *)e {
	self.inside = YES;
	[self performSelector:@selector(show) withObject:nil afterDelay:0.25];
}

- (void)mouseExited:(NSEvent *)e {
	self.inside = NO;
	[NSObject cancelPreviousPerformRequestsWithTarget:self selector:@selector(show) object:nil];
	[self hide];
}

- (void)show {
	if (!self.inside || self.menuOpen) return;
	if (!self.panel) [self build];
	[self tick];
	NSRect a = self.anchor.frame;
	NSRect s = (self.anchor.screen ?: NSScreen.mainScreen).visibleFrame;
	NSSize z = self.panel.frame.size;
	// centred on the screen, just under the menu bar
	[self.panel setFrameOrigin:NSMakePoint(round(NSMidX(s) - z.width / 2), MIN(NSMinY(a), NSMaxY(s)) - z.height - 6)];
	[self.panel orderFrontRegardless];
	[self.timer invalidate];
	self.timer = [NSTimer scheduledTimerWithTimeInterval:1.0 / 10 target:self selector:@selector(tick) userInfo:nil repeats:YES];
	[[NSRunLoop currentRunLoop] addTimer:self.timer forMode:NSRunLoopCommonModes];
}

- (void)hide {
	[self.timer invalidate];
	self.timer = nil;
	[self.panel orderOut:nil];
}

- (void)tick {
	int n = 0;
	void *p = mqPreviewFrame(&n);
	if (p && n > 0) {
		NSImage *img = [[NSImage alloc] initWithData:[NSData dataWithBytesNoCopy:p length:n freeWhenDone:YES]];
		if (img) self.view.image = img;
	} else if (p) {
		free(p);
	}
	char *c = mqPreviewTitle();
	if (c) {
		self.caption.stringValue = [NSString stringWithUTF8String:c];
		free(c);
	}
}

- (void)menuBegan:(NSNotification *)n {
	self.menuOpen = YES;
	[self hide];
}

- (void)menuEnded:(NSNotification *)n {
	self.menuOpen = NO;
}

@end

static MQHover *hover;

static void tryAttach(int tries) {
	if ([hover attach] || tries <= 0) return;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		tryAttach(tries - 1);
	});
}

void mqStartHover(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (hover) return;
		hover = [MQHover new];
		NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
		[nc addObserver:hover selector:@selector(menuBegan:) name:NSMenuDidBeginTrackingNotification object:nil];
		[nc addObserver:hover selector:@selector(menuEnded:) name:NSMenuDidEndTrackingNotification object:nil];
		tryAttach(20);
	});
}
