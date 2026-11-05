import SwiftUI

/// How full a machine's disk is. `disk` is nil until the region's reconciler has reported one.
struct DiskBar: View {
    let disk: DiskUsage?
    let label: String

    private static func human(_ bytes: Double) -> String {
        let gb = 1024.0 * 1024 * 1024
        return bytes >= gb ? String(format: "%.1f GB", bytes / gb) : String(format: "%.0f MB", bytes / (1024 * 1024))
    }

    static func color(_ level: String) -> Color {
        switch level { case "critical": return .red; case "warn": return .orange; default: return .green }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Label(label, systemImage: "internaldrive").font(.caption).foregroundStyle(.secondary)
                Spacer()
                if let d = disk {
                    Text("\(Int(d.usedPercent.rounded()))% used")
                        .font(.caption.weight(d.filling ? .semibold : .regular))
                        .foregroundStyle(d.filling ? Self.color(d.level) : .primary)
                } else {
                    Text("not reported yet").font(.caption).foregroundStyle(.secondary)
                }
            }
            if let d = disk {
                ProgressView(value: min(max(d.usedPercent, 0), 100), total: 100)
                    .tint(Self.color(d.level))
                Text("\(Self.human(d.freeBytes)) free of \(Self.human(d.totalBytes))")
                    .font(.caption2).foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
    }
}
