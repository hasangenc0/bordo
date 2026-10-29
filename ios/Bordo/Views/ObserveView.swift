import SwiftUI
import Charts

struct MetricSeries: Identifiable {
    let id = UUID()
    let label: String
    let points: [(date: Date, value: Double)]
}

struct LogLine: Identifiable {
    let id = UUID()
    let ts: Date
    let region: String
    let line: String
}

struct ObserveResult {
    var series: [MetricSeries] = []
    var lines: [LogLine] = []
    var unavailable: String?
    var errors: [String] = []
}

struct TimeRange: Hashable {
    let label: String
    let seconds: Int
    let step: Int
    static let all = [TimeRange(label: "1h", seconds: 3600, step: 30), TimeRange(label: "6h", seconds: 6 * 3600, step: 120),
                      TimeRange(label: "24h", seconds: 24 * 3600, step: 300), TimeRange(label: "3d", seconds: 3 * 24 * 3600, step: 900)]
}

extension AgentClient {
    func queryMetrics(_ promql: String, range: TimeRange, region: String?) async throws -> ObserveResult {
        let end = Int(Date().timeIntervalSince1970)
        var q = [URLQueryItem(name: "query", value: promql), URLQueryItem(name: "start", value: String(end - range.seconds)),
                 URLQueryItem(name: "end", value: String(end)), URLQueryItem(name: "step", value: "\(range.step)s")]
        if let region { q.append(URLQueryItem(name: "region", value: region)) }
        let raw = try await cp("/observe/metrics?" + Self.encode(q))
        if raw["status"].str == "backends_not_configured" {
            return ObserveResult(unavailable: raw["message"].string ?? "Observability backends are not configured")
        }
        var out = ObserveResult()
        let regions = raw.object ?? [:]
        for (name, resp) in regions.sorted(by: { $0.key < $1.key }) {
            if let e = resp["error"].string?.nonEmpty { out.errors.append("\(name): \(e)"); continue }
            let result = resp["data"]["result"].array
            for r in result {
                let pts = r["values"].array.compactMap { v -> (Date, Double)? in
                    guard let t = v[0].double, let n = v[1].double, n.isFinite else { return nil }
                    return (Date(timeIntervalSince1970: t), n)
                }
                guard !pts.isEmpty else { continue }
                let rest = (r["metric"].object ?? [:]).filter { !["__name__", "job", "instance"].contains($0.key) }
                    .sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value.str)" }.joined(separator: " ")
                let parts = [regions.count > 1 ? name : "", result.count > 1 ? rest : ""].filter { !$0.isEmpty }
                out.series.append(MetricSeries(label: parts.joined(separator: " · ").nonEmpty ?? name, points: pts))
            }
        }
        return out
    }

    func queryLogs(_ logql: String, range: TimeRange, region: String?) async throws -> ObserveResult {
        let endNs = UInt64(Date().timeIntervalSince1970 * 1e9)
        let startNs = endNs - UInt64(range.seconds) * 1_000_000_000
        var q = [URLQueryItem(name: "query", value: logql), URLQueryItem(name: "start", value: String(startNs)), URLQueryItem(name: "end", value: String(endNs))]
        if let region { q.append(URLQueryItem(name: "region", value: region)) }
        let raw = try await cp("/observe/logs?" + Self.encode(q))
        if raw["status"].str == "backends_not_configured" {
            return ObserveResult(unavailable: raw["message"].string ?? "Observability backends are not configured")
        }
        var out = ObserveResult()
        for (name, resp) in (raw.object ?? [:]).sorted(by: { $0.key < $1.key }) {
            if let e = resp["error"].string?.nonEmpty { out.errors.append("\(name): \(e)"); continue }
            for stream in resp["data"]["result"].array {
                for v in stream["values"].array {
                    guard let ns = Double(v[0].str) else { continue }
                    out.lines.append(LogLine(ts: Date(timeIntervalSince1970: ns / 1e9), region: name, line: v[1].str))
                }
            }
        }
        out.lines.sort { $0.ts > $1.ts }
        return out
    }

    static func encode(_ items: [URLQueryItem]) -> String {
        var c = URLComponents(); c.queryItems = items
        // URLComponents leaves '+' and '&' in values alone; PromQL/LogQL contain both.
        var allowed = CharacterSet.urlQueryAllowed; allowed.remove(charactersIn: "+&=")
        return items.map { "\($0.name.addingPercentEncoding(withAllowedCharacters: allowed) ?? "")=\($0.value?.addingPercentEncoding(withAllowedCharacters: allowed) ?? "")" }.joined(separator: "&")
    }
}

struct ObserveView: View {
    let client: AgentClient
    enum Mode: String, CaseIterable, Identifiable { case metrics = "Metrics", logs = "Logs"; var id: String { rawValue } }

    @State private var mode = Mode.metrics
    @State private var query = "up"
    @State private var range = TimeRange.all[0]
    @State private var region = ""
    @State private var regions: [Region] = []
    @State private var result = ObserveResult()
    @State private var ran = false
    @State private var running = false
    @State private var error: String?
    @FocusState private var focused: Bool

    private static let metricExamples = ["up", "sum(rate(http_server_request_duration_seconds_count[5m]))", "sum by (job) (rate(http_server_request_duration_seconds_count[5m]))"]
    private static let logExamples = ["{job=~\".+\"}", "{job=~\".+\"} |= \"error\""]
    private var examples: [String] { mode == .metrics ? Self.metricExamples : Self.logExamples }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Picker("Mode", selection: $mode) { ForEach(Mode.allCases) { Text($0.rawValue).tag($0) } }.pickerStyle(.segmented)
                        .onChange(of: mode) { _, m in query = (m == .metrics ? Self.metricExamples : Self.logExamples)[0]; result = ObserveResult(); ran = false; error = nil }

                    TextField(mode == .metrics ? "PromQL" : "LogQL", text: $query, axis: .vertical)
                        .lineLimit(1...5).focused($focused)
                        .font(.system(.footnote, design: .monospaced))
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                        .padding(10).background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 10))

                    HStack {
                        Picker("Range", selection: $range) { ForEach(TimeRange.all, id: \.self) { Text($0.label).tag($0) } }.pickerStyle(.segmented)
                    }
                    HStack {
                        Picker("Region", selection: $region) {
                            Text("All regions").tag("")
                            ForEach(regions) { Text($0.name).tag($0.name) }
                        }.pickerStyle(.menu)
                        Spacer()
                        Button { focused = false; Task { await run() } } label: { Text(running ? "Running…" : "Run").frame(minWidth: 70) }
                            .buttonStyle(.borderedProminent).disabled(running || query.trimmed.isEmpty)
                    }

                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack { ForEach(examples, id: \.self) { q in Button { query = q; Task { await run() } } label: { Text(q).font(.system(.caption2, design: .monospaced)).lineLimit(1) }.buttonStyle(.bordered).controlSize(.small) } }
                    }

                    if let error { ErrorBanner(message: error) }
                    if let u = result.unavailable {
                        Text("Observability isn't set up on this server. Deploy the observability stack and set each region's backend URLs.\n\(u)").font(.footnote).padding(10)
                            .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 10))
                    }
                    ForEach(result.errors, id: \.self) { ErrorBanner(message: $0) }

                    if result.unavailable == nil && ran {
                        if mode == .metrics { chart } else { logs }
                    }
                }
                .padding(16)
            }
            .scrollDismissesKeyboard(.interactively)
            .navigationTitle("Observe")
            .task { regions = (try? await client.regions()) ?? [] }
        }
    }

    @ViewBuilder private var chart: some View {
        if result.series.isEmpty { Text("No data in this range.").font(.footnote).foregroundStyle(.secondary) } else {
            Chart {
                ForEach(result.series) { s in
                    ForEach(Array(s.points.enumerated()), id: \.offset) { _, p in
                        LineMark(x: .value("Time", p.date), y: .value("Value", p.value), series: .value("Series", s.label))
                            .foregroundStyle(by: .value("Series", s.label))
                    }
                }
            }
            .chartLegend(result.series.count > 1 ? .visible : .hidden)
            .frame(height: 260)
            .padding(10).background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 12))
        }
    }

    @ViewBuilder private var logs: some View {
        if result.lines.isEmpty { Text("No log lines matched.").font(.footnote).foregroundStyle(.secondary) } else {
            LazyVStack(alignment: .leading, spacing: 4) {
                ForEach(result.lines.prefix(500)) { l in
                    (Text(l.ts, format: .dateTime.hour().minute().second()).foregroundColor(.secondary)
                        + Text(regions.count > 1 ? " [\(l.region)] " : " ").foregroundColor(.secondary)
                        + Text(l.line))
                        .font(.system(size: 11, design: .monospaced)).textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(10).background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 12))
        }
    }

    private func run() async {
        running = true; error = nil
        defer { running = false }
        do {
            result = mode == .metrics
                ? try await client.queryMetrics(query, range: range, region: region.nonEmpty)
                : try await client.queryLogs(query, range: range, region: region.nonEmpty)
            ran = true
        } catch { self.error = error.localizedDescription }
    }
}
