import Cocoa
import SwiftUI

// =============================================================================
// Input
// =============================================================================

let args = CommandLine.arguments
let input = DialogInput(
    tool: args.count > 1 ? args[1] : "Unknown",
    command: args.count > 2 ? args[2] : "",
    aiReason: args.count > 3 ? args[3] : "",
    userMessage: args.count > 4 ? args[4] : "",
    cwd: args.count > 5 ? args[5] : ""
)

// =============================================================================
// App + Panel setup
// =============================================================================

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

// Edit menu for Cmd+V/C/X/A
let mainMenu = NSMenu()
let editMenu = NSMenu(title: "Edit")
editMenu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
let editMenuItem = NSMenuItem()
editMenuItem.submenu = editMenu
mainMenu.addItem(editMenuItem)
app.mainMenu = mainMenu

let state = DialogState()

let hostingView = NSHostingView(rootView: DialogView(input: input, state: state))
hostingView.frame = NSRect(x: 0, y: 0, width: 400, height: 160)

let screen = NSScreen.main!
let screenFrame = screen.visibleFrame

let panel = NSPanel(
    contentRect: NSRect(x: 0, y: 0, width: 400, height: 160),
    styleMask: [.titled, .nonactivatingPanel, .fullSizeContentView, .resizable],
    backing: .buffered,
    defer: false
)
panel.title = ""
panel.level = .floating
panel.isMovableByWindowBackground = true
panel.titlebarAppearsTransparent = true
panel.titleVisibility = .hidden
panel.isOpaque = false
panel.backgroundColor = .clear
panel.hasShadow = true
panel.becomesKeyOnlyIfNeeded = true
panel.contentView = hostingView

// Position: top-right, 10% below top
let panelX = screenFrame.maxX - 416
let panelY = screenFrame.maxY - (screenFrame.height * 0.1) - 160
panel.setFrameOrigin(NSPoint(x: panelX, y: panelY))

// Show without stealing focus
panel.orderFrontRegardless()

// =============================================================================
// Global keyboard shortcuts
// =============================================================================

NSEvent.addGlobalMonitorForEvents(matching: .keyDown) { event in
    if event.modifierFlags.contains(.command) && event.keyCode == 36 {
        state.result = .approve; NSApp.stop(nil)
    }
    if event.keyCode == 53 {
        state.result = .later; NSApp.stop(nil)
    }
}

NSEvent.addLocalMonitorForEvents(matching: .keyDown) { event in
    if event.modifierFlags.contains(.command) && event.keyCode == 36 {
        state.result = .approve; NSApp.stop(nil)
        return nil
    }
    if event.keyCode == 53 {
        state.result = .later; NSApp.stop(nil)
        return nil
    }
    return event
}

// =============================================================================
// Observe state changes to resize panel
// =============================================================================

state.onExpandToggle = {
    DispatchQueue.main.async {
        let size = hostingView.fittingSize
        let top = panel.frame.maxY
        panel.setContentSize(NSSize(width: 400, height: size.height))
        panel.setFrameOrigin(NSPoint(x: panel.frame.origin.x, y: top - size.height))
    }
}

state.onDismiss = {
    NSApp.stop(nil)
}

// =============================================================================
// Run
// =============================================================================

app.run()

print(state.result.rawValue)
if !state.feedback.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
    print(state.feedback.trimmingCharacters(in: .whitespacesAndNewlines))
}
