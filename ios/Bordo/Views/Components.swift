import SwiftUI
import UIKit

struct StatusBadge: View {
    let status: String

    static func color(_ status: String) -> Color {
        switch status.lowercased() {
        case "running", "healthy", "success", "succeeded", "ready", "approved", "auto_approved": return .green
        case "deploying", "building", "pending", "queued", "in_progress", "progressing": return .orange
        case "failed", "error", "unhealthy", "rolled_back": return .red
        default: return .gray
        }
    }

    var body: some View {
        let c = Self.color(status)
        HStack(spacing: 5) {
            Circle().fill(c).frame(width: 6, height: 6)
            Text(status.isEmpty ? "unknown" : status.replacingOccurrences(of: "_", with: " "))
                .font(.caption2.weight(.medium))
                .foregroundStyle(c)
        }
        .padding(.horizontal, 9).padding(.vertical, 4)
        .background(c.opacity(0.12), in: Capsule())
    }
}

struct CopyButton: View {
    let text: String
    @State private var copied = false

    var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            UIPasteboard.general.string = text
            copied = true
            Task { try? await Task.sleep(for: .seconds(1.5)); copied = false }
        } label: {
            Label(copied ? "Copied" : "Copy", systemImage: copied ? "checkmark" : "doc.on.doc")
                .font(.caption)
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
    }
}

struct CodeBlock: View {
    let text: String
    var title: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(title ?? "").font(.caption).foregroundStyle(.secondary)
                Spacer()
                CopyButton(text: text)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                Text(text)
                    .font(.system(.footnote, design: .monospaced))
                    .textSelection(.enabled)
                    .fixedSize(horizontal: true, vertical: true)
            }
        }
        .padding(12)
        .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

struct ErrorBanner: View {
    let message: String
    var body: some View {
        Label(message, systemImage: "exclamationmark.triangle.fill")
            .font(.footnote)
            .foregroundStyle(.red)
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.red.opacity(0.1), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

struct EmptyStateView: View {
    let icon: String
    let title: String
    var hint: String = ""
    var body: some View {
        ContentUnavailableView(title, systemImage: icon, description: hint.isEmpty ? nil : Text(hint))
    }
}

/// Loads async data, refreshes on pull and on a timer, and keeps the last good value on failure.
@MainActor
final class Loader<T>: ObservableObject {
    @Published private(set) var value: T?
    @Published private(set) var error: String?
    @Published private(set) var loading = false
    private let fetch: () async throws -> T

    init(_ fetch: @escaping () async throws -> T) { self.fetch = fetch }

    func reload() async {
        loading = true
        defer { loading = false }
        do { value = try await fetch(); error = nil }
        catch is CancellationError { }
        catch let e as URLError where e.code == .cancelled { }
        catch { self.error = error.localizedDescription }
    }

    /// Reloads now and every `interval` seconds until the view disappears.
    func poll(every interval: Double = 10) async {
        while !Task.isCancelled {
            await reload()
            try? await Task.sleep(for: .seconds(interval))
        }
    }
}

/// Round app/region glyph: first letter on the accent gradient.
struct Avatar: View {
    let name: String
    var size: CGFloat = 40
    var body: some View {
        Text(name.first.map { String($0).uppercased() } ?? "?")
            .font(.system(size: size * 0.42, weight: .bold, design: .rounded))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(Theme.gradient, in: RoundedRectangle(cornerRadius: size * 0.3, style: .continuous))
    }
}
