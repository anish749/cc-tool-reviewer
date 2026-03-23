import Foundation

struct DialogInput {
    let tool: String
    let command: String
    let aiReason: String
    let userMessage: String
    let cwd: String
}

enum Decision: String {
    case approve
    case deny
    case later
}

class DialogState: ObservableObject {
    @Published var isExpanded = false
    @Published var feedback = ""
    var result: Decision = .later
    var onExpandToggle: (() -> Void)?
    var onDismiss: (() -> Void)?

    func toggleExpand() {
        isExpanded.toggle()
        onExpandToggle?()
    }

    func approve() { result = .approve; onDismiss?() }
    func deny() { result = .deny; onDismiss?() }
    func later() { result = .later; onDismiss?() }
}
