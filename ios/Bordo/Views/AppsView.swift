import SwiftUI

struct AppsView: View {
    let client: AgentClient
    @StateObject private var apps: Loader<[Project]>
    @State private var creating = false
    @State private var deploying: Project?

    init(client: AgentClient) {
        self.client = client
        _apps = StateObject(wrappedValue: Loader { try await client.projects() })
    }

    var body: some View {
        NavigationStack {
            List {
                if let e = apps.error { ErrorBanner(message: e).listRowInsets(.init()) }
                ForEach(apps.value ?? []) { p in
                    NavigationLink(value: p) {
                        HStack(spacing: 12) {
                            Avatar(name: p.name)
                            VStack(alignment: .leading, spacing: 3) {
                                Text(p.name).font(.headline)
                                Text([p.template, p.releaseRegion].filter { !$0.isEmpty }.joined(separator: " · ").nonEmpty ?? "Not deployed")
                                    .font(.caption).foregroundStyle(.secondary).lineLimit(1)
                            }
                            Spacer(minLength: 8)
                            StatusBadge(status: p.releaseStatus.nonEmpty ?? "unknown")
                        }
                    }
                    .buttonStyle(.plain)
                    .cardRow()
                    .swipeActions { Button { deploying = p } label: { Label("Deploy", systemImage: "paperplane") }.tint(Theme.accent) }
                }
            }
            .listStyle(.plain).screenBackground()
            .overlay {
                if (apps.value ?? []).isEmpty && apps.error == nil && !apps.loading {
                    EmptyStateView(icon: "shippingbox", title: "No apps yet", hint: "Create one, or ask the agent: “create a java web service called hello-api”.")
                }
            }
            .navigationTitle("Apps")
            .navigationDestination(for: Project.self) { AppDetailView(client: client, project: $0) }
            .toolbar { ToolbarItem(placement: .primaryAction) { Button { creating = true } label: { Image(systemName: "plus") } } }
            .refreshable { await apps.reload() }
            .task { await apps.poll(every: 10) }
            .sheet(isPresented: $creating) { NewAppView(client: client) { await apps.reload() } }
            .sheet(item: $deploying) { DeployView(client: client, project: $0) { await apps.reload() } }
        }
    }
}

struct AppDetailView: View {
    let client: AgentClient
    let project: Project
    @EnvironmentObject var router: Router
    @StateObject private var releases: Loader<[Release]>
    @StateObject private var builds: Loader<[Build]>
    @State private var deploying = false
    @State private var logTitle = ""
    @State private var logText: String?
    @State private var rollback: Release?
    @State private var error: String?
    @State private var building = false

    init(client: AgentClient, project: Project) {
        self.client = client
        self.project = project
        _releases = StateObject(wrappedValue: Loader { try await client.releases(project: project.id) })
        _builds = StateObject(wrappedValue: Loader { try await client.builds(project: project.id) })
    }

    var body: some View {
        List {
            if let error { ErrorBanner(message: error).listRowInsets(.init()) }

            Section {
                LabeledContent("Template", value: project.template.nonEmpty ?? "—")
                if !project.gitRepoURL.isEmpty { LabeledContent("Repo") { Text(project.gitRepoURL).font(.caption).textSelection(.enabled) } }
                Button { router.ask("Change the code of \(project.name): ") } label: { Label("Ask the agent to change the code", systemImage: "sparkles") }
            }

            Section("Releases") {
                Button { deploying = true } label: { Label("Deploy…", systemImage: "paperplane.fill") }
                if (releases.value ?? []).isEmpty { Text("Nothing deployed yet. Build the app, then deploy.").font(.footnote).foregroundStyle(.secondary) }
                ForEach(releases.value ?? []) { r in
                    Button { Task { await showLog(title: "\(r.region) release", path: "/releases/\(r.id)/logs", key: "log") } } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(r.region).foregroundStyle(.primary)
                                Text("\(r.imageTag) · \(r.strategy) · \(Dates.relative(r.createdAt))").font(.caption).foregroundStyle(.secondary).lineLimit(1)
                            }
                            Spacer()
                            StatusBadge(status: r.status)
                        }
                    }
                    .swipeActions { Button { rollback = r } label: { Label("Roll back", systemImage: "arrow.uturn.backward") }.tint(.orange) }
                }
            }

            Section("Builds") {
                Button { Task { await startBuild() } } label: {
                    HStack { Label("Build", systemImage: "hammer.fill"); if building { Spacer(); ProgressView() } }
                }.disabled(building)
                ForEach(builds.value ?? []) { b in
                    Button { Task { await showBuildLog(b) } } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(b.imageTag.nonEmpty ?? b.id.prefix(8).description).foregroundStyle(.primary).lineLimit(1)
                                Text(b.error.nonEmpty ?? Dates.relative(b.createdAt)).font(.caption).foregroundStyle(b.error.isEmpty ? Color.secondary : Color.red).lineLimit(2)
                            }
                            Spacer()
                            StatusBadge(status: b.status)
                        }
                    }
                }
            }
        }
        .listStyle(.insetGrouped).screenBackground()
        .navigationTitle(project.name)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await releases.reload(); await builds.reload() }
        .task { await releases.poll(every: 8) }
        .task { await builds.poll(every: 8) }
        .sheet(isPresented: $deploying) { DeployView(client: client, project: project) { await releases.reload() } }
        .sheet(isPresented: Binding(get: { logText != nil }, set: { if !$0 { logText = nil } })) {
            NavigationStack {
                ScrollView { Text(logText ?? "").font(.system(.caption, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading).padding() }
                    .navigationTitle(logTitle).navigationBarTitleDisplayMode(.inline)
                    .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { logText = nil } } }
            }
        }
        .confirmationDialog("Roll back this release?", isPresented: Binding(get: { rollback != nil }, set: { if !$0 { rollback = nil } }), titleVisibility: .visible) {
            Button("Roll back", role: .destructive) {
                if let r = rollback { Task { await doRollback(r) } }
            }
        } message: { Text("The previous release becomes active again.") }
    }

    private func showLog(title: String, path: String, key: String) async {
        logTitle = title; logText = "Loading…"
        do { logText = try await client.cp(path)[key].str.nonEmpty ?? "No output yet." } catch { logText = error.localizedDescription }
    }

    private func showBuildLog(_ b: Build) async {
        logTitle = "Build log"; logText = "Loading…"
        do {
            let lines = try await client.cp("/builds/\(b.id)/logs")["lines"].array
            logText = lines.map { $0.string ?? $0.first("line", "message", "text").str }.joined(separator: "\n").nonEmpty ?? "No output yet."
        } catch { logText = error.localizedDescription }
    }

    private func startBuild() async {
        building = true; error = nil
        defer { building = false }
        do { try await client.cp("/builds", method: "POST", body: ["project_id": .string(project.id)]); await builds.reload() }
        catch { self.error = error.localizedDescription }
    }

    private func doRollback(_ r: Release) async {
        error = nil
        do { try await client.cp("/releases/\(r.id)/rollback", method: "POST", body: [:]); await releases.reload() }
        catch { self.error = error.localizedDescription }
    }
}

struct DeployView: View {
    let client: AgentClient
    let project: Project
    let onDone: () async -> Void
    @Environment(\.dismiss) private var dismiss

    @State private var regions: [Region] = []
    @State private var builds: [Build] = []
    @State private var picked: Set<String> = []
    @State private var image = ""
    @State private var strategy = "rolling"
    @State private var expose = "none"
    @State private var host = ""
    @State private var submitting = false
    @State private var error: String?

    private var built: [Build] { builds.filter { $0.status == "success" && !$0.imageTag.isEmpty } }
    private var effectiveImage: String { image.nonEmpty ?? built.first?.imageTag ?? "" }
    private var problem: String? {
        if picked.isEmpty { return "Pick at least one region." }
        if effectiveImage.isEmpty { return "Choose or enter an image to deploy." }
        if expose == "ingress" && host.trimmed.isEmpty { return "Ingress needs a hostname." }
        return nil
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Regions") {
                    ForEach(regions.filter { $0.status == "healthy" }) { r in
                        Toggle(r.name, isOn: Binding(get: { picked.contains(r.name) }, set: { if $0 { picked.insert(r.name) } else { picked.remove(r.name) } }))
                    }
                    if !regions.contains(where: { $0.status == "healthy" }) { Text("No healthy regions yet.").foregroundStyle(.secondary) }
                }
                Section("Image") {
                    if !built.isEmpty {
                        Picker("Built image", selection: $image) {
                            ForEach(built) { Text($0.imageTag).tag($0.imageTag) }
                        }
                    }
                    TextField("Or enter an image tag", text: $image).textInputAutocapitalization(.never).autocorrectionDisabled()
                }
                Section("Rollout") {
                    Picker("Strategy", selection: $strategy) {
                        Text("Rolling").tag("rolling"); Text("Canary").tag("canary"); Text("Blue/green").tag("blue-green")
                    }
                    Picker("Expose", selection: $expose) {
                        Text("Internal only").tag("none"); Text("Ingress").tag("ingress")
                    }
                    if expose == "ingress" { TextField("Hostname", text: $host).textInputAutocapitalization(.never).autocorrectionDisabled() }
                }
                if let e = error ?? problem { Section { if error != nil { ErrorBanner(message: e) } else { Text(e).font(.footnote).foregroundStyle(.secondary) } } }
            }
            .navigationTitle("Deploy \(project.name)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Deploy") { Task { await submit() } }.disabled(problem != nil || submitting)
                }
            }
            .task {
                regions = (try? await client.regions()) ?? []
                builds = (try? await client.builds(project: project.id)) ?? []
                if image.isEmpty { image = built.first?.imageTag ?? "" }
            }
        }
    }

    private func submit() async {
        submitting = true; error = nil
        var body: [String: JSON] = [
            "project_id": .string(project.id), "image_tag": .string(effectiveImage),
            "regions": .array(picked.sorted().map { .string($0) }), "strategy": .string(strategy), "expose": .string(expose),
        ]
        if expose == "ingress" { body["expose_host"] = .string(host.trimmed) }
        do {
            try await client.cp("/releases", method: "POST", body: .object(body))
            await onDone(); dismiss()
        } catch { self.error = error.localizedDescription; submitting = false }
    }
}

struct NewAppView: View {
    let client: AgentClient
    let onCreated: () async -> Void
    @Environment(\.dismiss) private var dismiss

    enum Source: String, CaseIterable, Identifiable { case template = "Template", repo = "GitHub repo"; var id: String { rawValue } }
    @State private var source = Source.template
    @State private var templates: [Template] = []
    @State private var template = ""
    @State private var name = ""
    @State private var repo = ""
    @State private var submitting = false
    @State private var error: String?

    private var nameOK: Bool { name.range(of: #"^[a-z][a-z0-9-]{1,38}[a-z0-9]$"#, options: .regularExpression) != nil }
    private var repoOK: Bool { repo.trimmed.range(of: #"^(https://github\.com/|git@github\.com:)[\w.-]+/[\w.-]+?(\.git)?/?$"#, options: .regularExpression) != nil }
    private var canSubmit: Bool { source == .template ? nameOK && !(template.nonEmpty ?? templates.first?.name ?? "").isEmpty : repoOK && (name.isEmpty || nameOK) }

    var body: some View {
        NavigationStack {
            Form {
                Picker("Start from", selection: $source) { ForEach(Source.allCases) { Text($0.rawValue).tag($0) } }.pickerStyle(.segmented)
                if source == .template {
                    Section("Template") {
                        Picker("Template", selection: $template) { ForEach(templates) { Text($0.name).tag($0.name) } }
                        if let t = templates.first(where: { $0.name == (template.nonEmpty ?? templates.first?.name) }), !t.description.isEmpty {
                            Text(t.description).font(.footnote).foregroundStyle(.secondary)
                        }
                    }
                } else {
                    Section("Repository") { TextField("https://github.com/owner/repo", text: $repo).textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL) }
                }
                Section { TextField(source == .repo ? "Name (optional)" : "Name", text: $name).textInputAutocapitalization(.never).autocorrectionDisabled() } footer: {
                    Text("Lowercase letters, digits and dashes.")
                }
                if let error { Section { ErrorBanner(message: error) } }
            }
            .navigationTitle("New app").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Create") { Task { await submit() } }.disabled(!canSubmit || submitting) }
            }
            .task {
                templates = (try? await client.templates()) ?? []
                if template.isEmpty { template = templates.first?.name ?? "" }
            }
        }
    }

    private func submit() async {
        submitting = true; error = nil
        let body: JSON = source == .repo
            ? (name.isEmpty ? ["git_repo_url": .string(repo.trimmed)] : ["git_repo_url": .string(repo.trimmed), "name": .string(name)])
            : ["name": .string(name), "template": .string(template.nonEmpty ?? templates.first?.name ?? "")]
        do { try await client.cp("/projects", method: "POST", body: body); await onCreated(); dismiss() }
        catch { self.error = error.localizedDescription; submitting = false }
    }
}
