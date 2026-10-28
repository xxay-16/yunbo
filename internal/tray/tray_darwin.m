#import <Cocoa/Cocoa.h>
#import "_cgo_export.h"

// Do not replace NSApp's delegate or start a second event loop: Wails owns both.
@interface YunboStatusMenuTarget : NSObject
- (void)handleAction:(NSMenuItem *)sender;
@end

@implementation YunboStatusMenuTarget
- (void)handleAction:(NSMenuItem *)sender {
    yunboTrayDispatch((int)sender.tag);
}
@end

static NSStatusItem *yunboStatusItem = nil;
static YunboStatusMenuTarget *yunboStatusTarget = nil;

static NSMenuItem *yunboMenuItem(NSString *title, NSInteger tag) {
    NSMenuItem *item = [[[NSMenuItem alloc] initWithTitle:title
        action:@selector(handleAction:) keyEquivalent:@""] autorelease];
    item.target = yunboStatusTarget;
    item.tag = tag;
    return item;
}

void yunboTrayStart(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            if (yunboStatusItem != nil) return;
            yunboStatusTarget = [[YunboStatusMenuTarget alloc] init];
            yunboStatusItem = [[[NSStatusBar systemStatusBar]
                statusItemWithLength:NSSquareStatusItemLength] retain];
            NSImage *image = [NSImage imageWithSystemSymbolName:@"icloud"
                accessibilityDescription:@"云泊"];
            image.template = YES;
            yunboStatusItem.button.image = image;
            if (image == nil) yunboStatusItem.button.title = @"云";
            yunboStatusItem.button.toolTip = @"云泊 · Yunbo";
            NSMenu *menu = [[[NSMenu alloc] initWithTitle:@"云泊"] autorelease];
            menu.autoenablesItems = NO;
            [menu addItem:yunboMenuItem(@"显示主窗口", 1)];
            [menu addItem:yunboMenuItem(@"收进后台", 2)];
            [menu addItem:yunboMenuItem(@"打开配置目录", 3)];
            [menu addItem:[NSMenuItem separatorItem]];
            [menu addItem:yunboMenuItem(@"退出云泊", 4)];
            yunboStatusItem.menu = menu;
            if (yunboStatusItem.button != nil) yunboTrayDispatch(0);
        }
    });
}

void yunboTrayStop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (yunboStatusItem != nil) {
            yunboStatusItem.menu = nil;
            [[NSStatusBar systemStatusBar] removeStatusItem:yunboStatusItem];
            [yunboStatusItem release];
            yunboStatusItem = nil;
        }
        [yunboStatusTarget release];
        yunboStatusTarget = nil;
    });
}
