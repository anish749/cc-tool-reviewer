import SwiftUI

struct DialogView: View {
    let input: DialogInput
    @ObservedObject var state: DialogState

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            compactSection
            if state.isExpanded {
                detailSection
            }
            buttonSection
        }
        .frame(width: 400)
        .background(.ultraThinMaterial)
        .overlay(accentBar, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    // =========================================================================
    // Compact section (always visible, clickable to toggle details)
    // =========================================================================

    var compactSection: some View {
        VStack(alignment: .leading, spacing: 3) {
            if !input.cwd.isEmpty {
                Text(input.cwd)
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(.white.opacity(0.4))
            }

            Text(input.tool)
                .font(.system(size: 13, weight: .bold))
                .foregroundStyle(.orange)

            if !input.command.isEmpty {
                Text(input.command)
                    .font(.system(size: 10.5, design: .monospaced))
                    .foregroundStyle(.white.opacity(0.85))
                    .lineLimit(2)
            }

            Text(state.isExpanded ? "▾ click to collapse" : "▸ click for details")
                .font(.system(size: 9))
                .foregroundStyle(.white.opacity(0.25))
        }
        .padding(.horizontal, 20)
        .padding(.top, 12)
        .padding(.bottom, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .onTapGesture {
            state.toggleExpand()
        }
    }

    // =========================================================================
    // Detail section (shown when expanded)
    // =========================================================================

    var detailSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            Divider().opacity(0.3)

            if !input.aiReason.isEmpty {
                sectionHeader("AI REASON")
                Text(input.aiReason)
                    .font(.system(size: 10))
                    .foregroundStyle(.white.opacity(0.5))
                    .lineLimit(2)
            }

            if !input.userMessage.isEmpty {
                sectionHeader("CONTEXT")
                Text(input.userMessage)
                    .font(.system(size: 10))
                    .foregroundStyle(.white.opacity(0.5))
                    .lineLimit(6)
            }

            sectionHeader("FEEDBACK (sent to Claude on deny)")
            TextEditor(text: $state.feedback)
                .font(.system(size: 10.5))
                .foregroundStyle(.white)
                .scrollContentBackground(.hidden)
                .background(.white.opacity(0.05))
                .clipShape(RoundedRectangle(cornerRadius: 4))
                .overlay(
                    RoundedRectangle(cornerRadius: 4)
                        .stroke(.white.opacity(0.1), lineWidth: 0.5)
                )
                .frame(height: 50)
        }
        .padding(.horizontal, 20)
        .padding(.bottom, 6)
        .transition(.identity)
    }

    // =========================================================================
    // Buttons (always visible)
    // =========================================================================

    var buttonSection: some View {
        HStack(spacing: 8) {
            actionButton("Deny", color: .red) { state.deny() }
            actionButton("Later", color: .gray) { state.later() }
            actionButton("Approve", color: .green) { state.approve() }
        }
        .padding(.horizontal, 20)
        .padding(.top, 4)
        .padding(.bottom, 12)
    }

    // =========================================================================
    // Reusable components
    // =========================================================================

    var accentBar: some View {
        RoundedRectangle(cornerRadius: 2)
            .fill(.orange)
            .frame(width: 3)
            .padding(.vertical, 8)
            .padding(.leading, 6)
    }

    func sectionHeader(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 9, weight: .bold))
            .foregroundStyle(.white.opacity(0.35))
    }

    func actionButton(_ title: String, color: Color, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(.system(size: 12, weight: .bold))
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.bordered)
        .tint(color)
    }
}
