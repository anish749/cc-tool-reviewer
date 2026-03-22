#!/usr/bin/env swift

import Cocoa

// Read arguments: tool, command, aiReason, userMessage, cwd
let args = CommandLine.arguments
let tool = args.count > 1 ? args[1] : "Unknown"
let command = args.count > 2 ? args[2] : ""
let aiReason = args.count > 3 ? args[3] : ""
let userMessage = args.count > 4 ? args[4] : ""
let cwd = args.count > 5 ? args[5] : ""

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

// Add Edit menu so Cmd+V/C/X/A work in the text field
let mainMenu = NSMenu()
let editMenuItem = NSMenuItem()
editMenuItem.submenu = {
    let menu = NSMenu(title: "Edit")
    menu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
    menu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
    menu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
    menu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
    return menu
}()
mainMenu.addItem(editMenuItem)
app.mainMenu = mainMenu

// Create the panel — floating, translucent
let panel = NSPanel(
    contentRect: NSRect(x: 0, y: 0, width: 520, height: 0),
    styleMask: [.titled, .nonactivatingPanel, .fullSizeContentView],
    backing: .buffered,
    defer: false
)
panel.title = "cc-tool-reviewer"
panel.level = .floating
panel.isMovableByWindowBackground = true
panel.titlebarAppearsTransparent = true
panel.titleVisibility = .hidden
panel.isOpaque = false
panel.backgroundColor = .clear

// Visual effect view — ultra-translucent glass
let visualEffect = NSVisualEffectView()
visualEffect.material = .underWindowBackground
visualEffect.state = .active
visualEffect.blendingMode = .behindWindow
visualEffect.wantsLayer = true
visualEffect.layer?.cornerRadius = 12
visualEffect.alphaValue = 0.82

// Content stack
let stack = NSStackView()
stack.orientation = .vertical
stack.alignment = .leading
stack.spacing = 12
stack.edgeInsets = NSEdgeInsets(top: 20, left: 24, bottom: 20, right: 24)

// Helper to create labels
func makeLabel(_ text: String, size: CGFloat, bold: Bool = false, color: NSColor = .labelColor, selectable: Bool = false) -> NSTextField {
    let label = NSTextField(wrappingLabelWithString: text)
    label.font = bold ? .boldSystemFont(ofSize: size) : .systemFont(ofSize: size)
    label.textColor = color
    label.isSelectable = selectable
    label.drawsBackground = false
    label.isBordered = false
    label.lineBreakMode = .byWordWrapping
    label.maximumNumberOfLines = 0
    label.preferredMaxLayoutWidth = 472
    return label
}

func makeSeparator() -> NSBox {
    let sep = NSBox()
    sep.boxType = .separator
    sep.translatesAutoresizingMaskIntoConstraints = false
    return sep
}

// Title
let titleLabel = makeLabel("Tool Review", size: 16, bold: true, color: .white)
stack.addArrangedSubview(titleLabel)

// CWD
if !cwd.isEmpty {
    let cwdLabel = makeLabel("📁 " + cwd, size: 11, color: NSColor.white.withAlphaComponent(0.4))
    cwdLabel.font = NSFont.monospacedSystemFont(ofSize: 11, weight: .regular)
    stack.addArrangedSubview(cwdLabel)
}

stack.addArrangedSubview(makeSeparator())

// User message context
if !userMessage.isEmpty {
    let contextHeader = makeLabel("USER REQUEST", size: 10, bold: true, color: NSColor.white.withAlphaComponent(0.5))
    stack.addArrangedSubview(contextHeader)

    let contextLabel = makeLabel(userMessage, size: 13, color: NSColor.white.withAlphaComponent(0.85))
    stack.addArrangedSubview(contextLabel)

    stack.addArrangedSubview(makeSeparator())
}

// Tool
let toolHeader = makeLabel("TOOL", size: 10, bold: true, color: NSColor.white.withAlphaComponent(0.5))
stack.addArrangedSubview(toolHeader)
let toolLabel = makeLabel(tool, size: 14, bold: true, color: .systemOrange)
stack.addArrangedSubview(toolLabel)

// Command
if !command.isEmpty {
    let cmdHeader = makeLabel("COMMAND", size: 10, bold: true, color: NSColor.white.withAlphaComponent(0.5))
    stack.addArrangedSubview(cmdHeader)

    // Command in a monospace "code block" style
    let cmdLabel = makeLabel(command, size: 12, color: .white, selectable: true)
    cmdLabel.font = NSFont.monospacedSystemFont(ofSize: 12, weight: .regular)

    let cmdContainer = NSView()
    cmdContainer.wantsLayer = true
    cmdContainer.layer?.backgroundColor = NSColor.white.withAlphaComponent(0.08).cgColor
    cmdContainer.layer?.cornerRadius = 6

    cmdLabel.translatesAutoresizingMaskIntoConstraints = false
    cmdContainer.addSubview(cmdLabel)
    NSLayoutConstraint.activate([
        cmdLabel.topAnchor.constraint(equalTo: cmdContainer.topAnchor, constant: 8),
        cmdLabel.bottomAnchor.constraint(equalTo: cmdContainer.bottomAnchor, constant: -8),
        cmdLabel.leadingAnchor.constraint(equalTo: cmdContainer.leadingAnchor, constant: 10),
        cmdLabel.trailingAnchor.constraint(equalTo: cmdContainer.trailingAnchor, constant: -10),
    ])
    cmdContainer.translatesAutoresizingMaskIntoConstraints = false

    stack.addArrangedSubview(cmdContainer)
    cmdContainer.widthAnchor.constraint(equalToConstant: 472).isActive = true
}

// AI reason
if !aiReason.isEmpty {
    let reasonHeader = makeLabel("AI REASON", size: 10, bold: true, color: NSColor.white.withAlphaComponent(0.5))
    stack.addArrangedSubview(reasonHeader)
    let reasonLabel = makeLabel(aiReason, size: 12, color: NSColor.white.withAlphaComponent(0.7))
    stack.addArrangedSubview(reasonLabel)
}

stack.addArrangedSubview(makeSeparator())

// Feedback text field
let feedbackHeader = makeLabel("FEEDBACK (optional)", size: 10, bold: true, color: NSColor.white.withAlphaComponent(0.5))
stack.addArrangedSubview(feedbackHeader)

let feedbackField = NSTextField()
feedbackField.placeholderString = "When denying, tell Claude what to do instead"
feedbackField.font = .systemFont(ofSize: 12)
feedbackField.textColor = .white
feedbackField.backgroundColor = NSColor.white.withAlphaComponent(0.08)
feedbackField.isEditable = true
feedbackField.isSelectable = true
feedbackField.isBordered = false
feedbackField.isBezeled = true
feedbackField.bezelStyle = .roundedBezel
feedbackField.focusRingType = .none
feedbackField.translatesAutoresizingMaskIntoConstraints = false
feedbackField.preferredMaxLayoutWidth = 472
stack.addArrangedSubview(feedbackField)
feedbackField.widthAnchor.constraint(equalToConstant: 472).isActive = true

// Buttons
let buttonStack = NSStackView()
buttonStack.orientation = .horizontal
buttonStack.spacing = 10
buttonStack.distribution = .fillEqually

func makeButton(_ title: String, color: NSColor, tag: Int) -> NSButton {
    let btn = NSButton(title: title, target: nil, action: nil)
    btn.bezelStyle = .rounded
    btn.isBordered = true
    btn.wantsLayer = true
    btn.layer?.cornerRadius = 6
    btn.contentTintColor = color
    btn.font = .boldSystemFont(ofSize: 13)
    btn.tag = tag
    btn.target = app
    btn.action = #selector(NSApplication.terminate(_:))
    return btn
}

let denyBtn = makeButton("Deny", color: .systemRed, tag: 2)
let laterBtn = makeButton("Later", color: .systemGray, tag: 3)
let approveBtn = makeButton("Approve", color: .systemGreen, tag: 1)

buttonStack.addArrangedSubview(denyBtn)
buttonStack.addArrangedSubview(laterBtn)
buttonStack.addArrangedSubview(approveBtn)

buttonStack.translatesAutoresizingMaskIntoConstraints = false
stack.addArrangedSubview(buttonStack)
buttonStack.widthAnchor.constraint(equalToConstant: 472).isActive = true

// Track which button was clicked
class ButtonHandler: NSObject {
    var result = "later"
    var feedback: String { feedbackField.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) }

    @objc func approve(_ sender: Any?) { result = "approve"; NSApp.stop(nil) }
    @objc func deny(_ sender: Any?) { result = "deny"; NSApp.stop(nil) }
    @objc func later(_ sender: Any?) { result = "later"; NSApp.stop(nil) }
}

let handler = ButtonHandler()
approveBtn.target = handler
approveBtn.action = #selector(ButtonHandler.approve(_:))
denyBtn.target = handler
denyBtn.action = #selector(ButtonHandler.deny(_:))
laterBtn.target = handler
laterBtn.action = #selector(ButtonHandler.later(_:))

// Layout
stack.translatesAutoresizingMaskIntoConstraints = false
visualEffect.translatesAutoresizingMaskIntoConstraints = false

let contentView = NSView()
contentView.addSubview(visualEffect)
contentView.addSubview(stack)

NSLayoutConstraint.activate([
    visualEffect.topAnchor.constraint(equalTo: contentView.topAnchor),
    visualEffect.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
    visualEffect.leadingAnchor.constraint(equalTo: contentView.leadingAnchor),
    visualEffect.trailingAnchor.constraint(equalTo: contentView.trailingAnchor),

    stack.topAnchor.constraint(equalTo: contentView.topAnchor),
    stack.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
    stack.leadingAnchor.constraint(equalTo: contentView.leadingAnchor),
    stack.trailingAnchor.constraint(equalTo: contentView.trailingAnchor),
])

panel.contentView = contentView

// Size to fit content
stack.layoutSubtreeIfNeeded()
let fittingSize = stack.fittingSize
panel.setContentSize(NSSize(width: 520, height: fittingSize.height))

// Center on screen
panel.center()

// Show and run
panel.makeKeyAndOrderFront(nil)
app.activate(ignoringOtherApps: true)
panel.makeFirstResponder(feedbackField)

// Keyboard shortcuts — Cmd+Enter = Approve, Escape = Later
approveBtn.keyEquivalent = "\r"
approveBtn.keyEquivalentModifierMask = [.command]
laterBtn.keyEquivalent = "\u{1b}"

app.run()

// Print result: "decision\nfeedback"
print(handler.result)
if !handler.feedback.isEmpty {
    print(handler.feedback)
}
