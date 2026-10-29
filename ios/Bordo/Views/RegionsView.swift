import SwiftUI

struct RegionView: Identifiable {
    var id: String { region.name }
    let region: Region
    let nodes: [NodeHealth]
    let deployments: [JSON]
}

struct RegionsView: View {
    let client: AgentClient
    @StateObject private var data: Loader<(views: [RegionView], names: [String: String])>
    @State private var adding = false
    @State private var joinFor: String?

    init(client: AgentClient) {
        self.client = client
        _data = StateObject(wrappedValue: Loader {
            async let regions = client.regions()
            async let fleet = (try? await client.cp("/fleet")) ?? .null
            async let projects = (try? await client.projects()) ?? []
            let (rs, fl, ps) = await (try regions, fleet, projects)
            var views: [RegionView] = []
            for r in rs {
                let nodes = (try? await client.nodes(region: r.name)) ?? []
                let deps = fl["regions"].array.first { $0["name"].str == r.name }?["deployments"].array ?? []
                views.append(RegionView(region: r, nodes: nodes, deployments: deps))
            }
            return (views, Dictionary(uniqueKeysWithValues: ps.map { ($0.id, $0.name) }))
        })
    }

    var body: some View {
        NavigationStack {
            List {
                if let e = data.error { ErrorBanner(message: e).listRowInsets(.init()) }
                Text("A region is a machine, or a small cluster, that runs your apps. Add one, run the command on it, and it appears here.")
                    .font(.footnote).foregroundStyle(.secondary)
                    .listRowInsets(EdgeInsets(top: 0, leading: 20, bottom: 4, trailing: 20)).listRowSeparator(.hidden).listRowBackground(Color.clear)
                ForEach(data.value?.views ?? []) { v in
                    VStack(alignment: .leading, spacing: 12) {
                        HStack(spacing: 12) {
                            Avatar(name: v.region.name, size: 36)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(v.region.name).font(.headline)
                                Text(summary(v)).font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer(minLength: 8)
                            StatusBadge(status: v.region.status)
                        }
                        if !v.nodes.isEmpty {
                            VStack(spacing: 6) {
                                ForEach(v.nodes) { n in
                                    HStack(spacing: 8) {
                                        Image(systemName: "server.rack").font(.caption).foregroundStyle(.secondary)
                                        Text(n.name).font(.subheadline)
                                        Spacer()
                                        if let hb = n.lastHeartbeat { Text(Dates.relative(hb)).font(.caption2).foregroundStyle(.secondary) }
                                        StatusBadge(status: n.ready ? "ready" : "not ready")
                                    }
                                }
                            }
                        }
                        if v.region.status != "healthy" {
                            Button { joinFor = v.region.name } label: { Label("Get join command", systemImage: "terminal").font(.subheadline) }
                                .buttonStyle(.bordered).controlSize(.small)
                        }
                        Divider()
                        if v.deployments.isEmpty {
                            Text("Nothing deployed here yet.").font(.footnote).foregroundStyle(.secondary)
                        } else {
                            VStack(spacing: 8) {
                                ForEach(Array(v.deployments.enumerated()), id: \.offset) { _, d in
                                    HStack {
                                        Image(systemName: "shippingbox").font(.caption).foregroundStyle(Theme.accent)
                                        Text(data.value?.names[d["project_id"].str] ?? String(d["project_id"].str.prefix(8))).font(.subheadline)
                                        Spacer(); StatusBadge(status: d["status"].str)
                                    }
                                }
                            }
                        }
                    }
                    .cardRow()
                }
            }
            .listStyle(.plain).screenBackground()
            .overlay { if (data.value?.views ?? []).isEmpty && data.error == nil && !data.loading { EmptyStateView(icon: "globe", title: "No regions yet", hint: "Add a region to get a place to deploy your apps.") } }
            .navigationTitle("Regions")
            .toolbar { ToolbarItem(placement: .primaryAction) { Button { adding = true } label: { Image(systemName: "plus") } } }
            .refreshable { await data.reload() }
            .task { await data.poll(every: 10) }
            .sheet(isPresented: $adding) { JoinRegionView(client: client, region: nil) { await data.reload() } }
            .sheet(item: Binding(get: { joinFor.map { Wrapped(value: $0) } }, set: { joinFor = $0?.value })) { w in
                JoinRegionView(client: client, region: w.value) { await data.reload() }
            }
        }
    }

    private struct Wrapped: Identifiable { let value: String; var id: String { value } }

    private func summary(_ v: RegionView) -> String {
        var s = v.nodes.isEmpty
            ? (v.region.status == "pending" ? "Waiting for a machine to join" : "No node reports yet")
            : "\(v.nodes.filter(\.ready).count)/\(v.nodes.count) node\(v.nodes.count == 1 ? "" : "s") ready"
        if let c = v.region.createdAt { s += " · added \(Dates.relative(c))" }
        return s
    }
}

struct JoinRegionView: View {
    let client: AgentClient
    let region: String?
    let onDone: () async -> Void
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject var connection: ConnectionManager
    @State private var name = ""
    @State private var cpURL = ""
    @State private var token: (value: String, expires: String)?
    @State private var busy = false
    @State private var error: String?

    private var regionName: String { region ?? name.trimmed }
    private var nameOK: Bool { regionName.range(of: #"^[a-z][a-z0-9-]{1,30}[a-z0-9]$"#, options: .regularExpression) != nil }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("http://server:7401", text: $cpURL).textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                } header: { Text("Control plane URL") } footer: { Text("The address the new machine will use to reach this server.") }
                if region == nil { Section("Region name") { TextField("us-east-1", text: $name).textInputAutocapitalization(.never).autocorrectionDisabled() } }
                if let token {
                    Section("Join command") {
                        CodeBlock(text: joinCommand(token.value), title: "Run on the target machine")
                        Text("Expires \(token.expires)").font(.caption).foregroundStyle(.secondary)
                    }
                } else {
                    Section { Button { Task { await create() } } label: { HStack { Text(region == nil ? "Add region" : "Create join command"); if busy { Spacer(); ProgressView() } } }.disabled(!nameOK || busy) }
                }
                if let error { Section { ErrorBanner(message: error) } }
            }
            .navigationTitle(region ?? "Add region").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { Task { await onDone(); dismiss() } } } }
            .onAppear { if cpURL.isEmpty { cpURL = "http://\(connection.profile?.host ?? "<server-ip>"):7401" } }
        }
    }

    private func joinCommand(_ token: String) -> String {
        let base = cpURL.trimmed.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return "curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \\\n  BORDO_CP_URL=\(base) \\\n  BORDO_JOIN_TOKEN=\(token) sh"
    }

    private func create() async {
        busy = true; error = nil
        defer { busy = false }
        do {
            if region == nil { try await client.cp("/regions", method: "POST", body: ["name": .string(regionName)]) }
            let r = try await client.cp("/regions/\(regionName)/join-tokens", method: "POST", body: [:])
            token = (r["token"].str, r["expires_at"].str)
        } catch { self.error = error.localizedDescription }
    }
}
