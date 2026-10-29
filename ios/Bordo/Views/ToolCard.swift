import SwiftUI

/// Collapsible record of one agent tool call, with a richer body for the calls
/// whose result the user usually wants to act on (PR links, join commands, diffs).
struct ToolCard: View {
    let tool: ToolActivity
    @State private var open = false

    private static let labels: [String: String] = [
        "code_checkout": "Checked out code", "code_list_files": "Listed files", "code_read_file": "Read",
        "code_write_file": "Wrote", "code_edit_file": "Edited", "code_search": "Searched code",
        "code_diff": "Reviewed changes", "code_commit_push": "Pushed branch", "code_open_pr": "Opened pull request",
        "project_create": "Created project", "project_list": "Listed projects", "build_trigger": "Started build",
        "build_status": "Checked build", "build_logs": "Fetched build logs", "deploy": "Deployed",
        "deploy_status": "Checked release", "deploy_logs": "Fetched deploy logs", "rollback": "Rolled back",
        "region_list": "Listed regions", "region_add": "Added region", "region_health": "Checked region health",
        "fleet_overview": "Fleet overview", "metrics_query": "Queried metrics", "logs_query": "Queried logs",
    ]
    private static let running: [String: String] = [
        "code_checkout": "Checking out code", "code_read_file": "Reading", "code_write_file": "Writing",
        "code_edit_file": "Editing", "code_commit_push": "Pushing branch", "deploy": "Deploying", "build_trigger": "Starting build",
    ]
    private static let alwaysOpen: Set<String> = ["code_diff", "code_commit_push", "code_open_pr", "region_add", "deploy", "rollback", "build_trigger"]

    private static func icon(_ name: String) -> String {
        switch true {
        case name.hasPrefix("code_"): return "chevron.left.forwardslash.chevron.right"
        case name.hasPrefix("build"): return "hammer"
        case name.hasPrefix("deploy"), name == "rollback": return "paperplane"
        case name.hasPrefix("region"), name.hasPrefix("fleet"): return "globe"
        case name.hasPrefix("metrics"), name.hasPrefix("logs"): return "chart.xyaxis.line"
        case name.hasPrefix("project"): return "shippingbox"
        default: return "checkmark.circle"
        }
    }

    private var title: String {
        let pretty = tool.name.replacingOccurrences(of: "_", with: " ")
        let base = tool.status == .running ? (Self.running[tool.name] ?? (Self.labels[tool.name] ?? pretty) + "…") : (Self.labels[tool.name] ?? pretty)
        let target = [tool.input["path"], tool.input["query"], tool.input["name"], tool.input["region"]].compactMap { $0.string?.nonEmpty }.first
        let full = target.map { "\(base) \($0)" } ?? base
        return tool.status == .error ? "\(full) — failed" : full
    }

    private var hasDetail: Bool { !tool.data.isNull || tool.input.object?.isEmpty == false || !tool.error.isEmpty }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button { if hasDetail { withAnimation { open.toggle() } } } label: {
                HStack(spacing: 8) {
                    switch tool.status {
                    case .running: ProgressView().controlSize(.small)
                    case .done: Image(systemName: Self.icon(tool.name)).foregroundStyle(Theme.accent).frame(width: 18)
                    case .error: Image(systemName: "xmark.circle.fill").foregroundStyle(.red)
                    }
                    Text(title).font(.footnote.weight(.medium)).foregroundStyle(.primary).lineLimit(2).multilineTextAlignment(.leading)
                    Spacer(minLength: 0)
                    if hasDetail { Image(systemName: open ? "chevron.down" : "chevron.right").font(.caption2).foregroundStyle(.secondary) }
                }
            }
            .buttonStyle(.plain)

            if tool.status == .error, !tool.error.isEmpty { Text(tool.error).font(.caption).foregroundStyle(.red) }
            if tool.status == .done, Self.alwaysOpen.contains(tool.name) { rich }
            if open {
                if tool.input.object?.isEmpty == false { CodeBlock(text: tool.input.pretty, title: "Input") }
                if !tool.data.isNull { CodeBlock(text: tool.data.pretty, title: "Result") }
            }
        }
        .padding(.horizontal, 12).padding(.vertical, 9)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Color(.separator).opacity(0.4), lineWidth: 0.5))
    }

    @ViewBuilder private var rich: some View {
        let d = tool.data
        switch tool.name {
        case "code_diff":
            row("Branch", d["branch"].str)
            if !d["diff"].str.isEmpty { DiffView(diff: d["diff"].str, truncated: d["truncated"].bool == true) }
        case "code_commit_push":
            row("Branch", d["branch"].str)
            row("Commit", String(d["commit"].str.prefix(8)))
            if let u = URL(string: d["compare_url"].str), !d["compare_url"].str.isEmpty { Link("View on GitHub", destination: u).font(.footnote) }
        case "code_open_pr":
            if let u = URL(string: d["url"].str), !d["url"].str.isEmpty { Link("Pull request #\(d["number"].str) on GitHub", destination: u).font(.footnote) }
        case "region_add":
            row("Region", d["region"].str)
            if !d["join_command"].str.isEmpty { CodeBlock(text: d["join_command"].str, title: "Run this on the target machine") }
        case "deploy", "deploy_status", "rollback":
            let releases = d["releases"].array.isEmpty ? [d] : d["releases"].array
            ForEach(Array(releases.enumerated()), id: \.offset) { _, r in
                HStack { Text(r["region"].str.nonEmpty ?? "release").font(.footnote); Spacer(); StatusBadge(status: r["status"].str) }
            }
        case "build_trigger", "build_status":
            HStack { Text("Status").font(.footnote); Spacer(); StatusBadge(status: d.first("status", "Status").str) }
            row("Image", d.first("image_tag", "ImageTag").str)
        default: EmptyView()
        }
    }

    @ViewBuilder private func row(_ k: String, _ v: String) -> some View {
        if !v.isEmpty {
            HStack { Text(k).font(.footnote).foregroundStyle(.secondary); Spacer(); Text(v).font(.system(.footnote, design: .monospaced)).textSelection(.enabled) }
        }
    }
}

struct DiffView: View {
    let diff: String
    let truncated: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            ScrollView([.horizontal, .vertical]) {
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(diff.components(separatedBy: "\n").enumerated()), id: \.offset) { _, line in
                        Text(line.isEmpty ? " " : line)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(color(line))
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(background(line))
                    }
                }
                .fixedSize(horizontal: true, vertical: false)
            }
            .frame(maxHeight: 280)
            if truncated { Text("Diff truncated").font(.caption2).foregroundStyle(.secondary) }
        }
        .padding(6)
        .background(Color(.tertiarySystemBackground), in: RoundedRectangle(cornerRadius: 8))
    }

    private func color(_ l: String) -> Color {
        if l.hasPrefix("+") && !l.hasPrefix("+++") { return .green }
        if l.hasPrefix("-") && !l.hasPrefix("---") { return .red }
        if l.hasPrefix("@@") { return .blue }
        return .primary
    }
    private func background(_ l: String) -> Color {
        if l.hasPrefix("+") && !l.hasPrefix("+++") { return .green.opacity(0.1) }
        if l.hasPrefix("-") && !l.hasPrefix("---") { return .red.opacity(0.1) }
        return .clear
    }
}
