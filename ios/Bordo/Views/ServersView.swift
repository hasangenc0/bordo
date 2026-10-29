import SwiftUI
import UniformTypeIdentifiers

struct ServersView: View {
    @EnvironmentObject var profiles: ProfileStore
    @EnvironmentObject var connection: ConnectionManager
    @State private var editing: ServerProfile?
    @State private var creating = false
    @AppStorage("bordo.lastProfile") private var lastProfile = ""
    @State private var triedAutoConnect = false

    var body: some View {
        NavigationStack {
            List {
                if case .failed(let msg) = connection.state {
                    ErrorBanner(message: msg).listRowInsets(EdgeInsets(top: 5, leading: 16, bottom: 5, trailing: 16)).listRowSeparator(.hidden).listRowBackground(Color.clear)
                }
                Group {
                    ForEach(profiles.profiles) { p in
                        Button { start(p) } label: {
                            HStack(spacing: 12) {
                                Avatar(name: p.displayName)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(p.displayName).font(.headline).foregroundStyle(.primary)
                                    Text("\(p.username)@\(p.host):\(p.sshPort)").font(.caption).foregroundStyle(.secondary)
                                }
                                Spacer()
                                if case .connecting(let step) = connection.state, connection.profile?.id == p.id {
                                    ProgressView()
                                    Text(step).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                                } else {
                                    Image(systemName: "chevron.right").font(.footnote).foregroundStyle(.tertiary)
                                }
                            }
                        }
                        .buttonStyle(.plain)
                        .cardRow()
                        .swipeActions(edge: .trailing) {
                            Button(role: .destructive) { profiles.delete(p) } label: { Label("Delete", systemImage: "trash") }
                            Button { editing = p } label: { Label("Edit", systemImage: "pencil") }.tint(.blue)
                        }
                    }
                }
                if !profiles.profiles.isEmpty {
                    Text("Swipe a server to edit or remove it.").font(.footnote).foregroundStyle(.secondary)
                        .listRowSeparator(.hidden).listRowBackground(Color.clear)
                }
            }
            .listStyle(.plain).screenBackground()
            .overlay { if profiles.profiles.isEmpty { EmptyStateView(icon: "server.rack", title: "No servers yet", hint: "Tap + and add the machine that runs Bordo. The app connects over SSH, so no ports need to be opened.") } }
            .navigationTitle("Bordo")
            .toolbar {
                ToolbarItem(placement: .primaryAction) { Button { creating = true } label: { Image(systemName: "plus") } }
            }
            .sheet(isPresented: $creating) { ServerEditView(profile: ServerProfile()) }
            .sheet(item: $editing) { ServerEditView(profile: $0) }
            .task {
                // Reopen straight into the last server, like the web console staying signed in.
                guard !triedAutoConnect else { return }
                triedAutoConnect = true
                if let p = profiles.profiles.first(where: { $0.id.uuidString == lastProfile }), connection.state == .disconnected { start(p) }
            }
        }
    }

    private func start(_ p: ServerProfile) {
        lastProfile = p.id.uuidString
        Task { await connection.connect(p) }
    }
}

struct ServerEditView: View {
    @EnvironmentObject var profiles: ProfileStore
    @EnvironmentObject var connection: ConnectionManager
    @Environment(\.dismiss) private var dismiss

    @State var profile: ServerProfile
    @State private var secrets: ServerSecrets
    @State private var portText: String
    @State private var agentPortText: String
    @State private var importing = false
    @State private var busy = false
    @State private var message: String?
    @State private var isError = false
    @State private var unknownKey: (fingerprint: String, openSSH: String)?

    init(profile: ServerProfile) {
        _profile = State(initialValue: profile)
        _secrets = State(initialValue: ServerSecrets.load(profile.id))
        _portText = State(initialValue: String(profile.sshPort))
        _agentPortText = State(initialValue: String(profile.agentPort))
    }

    private var publicLine: String? { SSHKeys.publicLine(stored: secrets.privateKey, passphrase: secrets.passphrase) }

    var body: some View {
        NavigationStack {
            Form {
                Section("Server") {
                    TextField("Name (optional)", text: $profile.name)
                    TextField("Host or IP", text: $profile.host)
                        .textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                    TextField("SSH port", text: $portText).keyboardType(.numberPad)
                    TextField("SSH user", text: $profile.username)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                }

                Section("Sign in") {
                    Picker("Method", selection: $profile.auth) {
                        ForEach(ServerProfile.Auth.allCases) { Text($0.rawValue).tag($0) }
                    }.pickerStyle(.segmented)

                    if profile.auth == .password {
                        SecureField("Password", text: $secrets.password)
                    } else {
                        keySection
                    }
                }

                Section {
                    SecureField("Agent token", text: $secrets.agentToken)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                    Button { Task { await fetchToken() } } label: {
                        HStack { Text("Read token from server"); if busy { Spacer(); ProgressView() } }
                    }
                    .disabled(busy || !canReach)
                } header: {
                    Text("Agent")
                } footer: {
                    Text("This is BORDO_INTERNAL_TOKEN from the server's deploy/bordo/.env. Leave blank if the agent runs without one.")
                }

                Section("Advanced") {
                    TextField("Agent port on the server", text: $agentPortText).keyboardType(.numberPad)
                    TextField("Bordo install directory", text: $profile.bordoDir)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                }

                if let message { Section { if isError { ErrorBanner(message: message) } else { Text(message).font(.footnote) } } }
            }
            .navigationTitle(profile.host.isEmpty ? "Add server" : profile.displayName)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Save") { save() }.disabled(!canSave) }
            }
            .alert("Trust this server?", isPresented: Binding(get: { unknownKey != nil }, set: { if !$0 { unknownKey = nil } })) {
                Button("Trust") {
                    if let k = unknownKey { HostKeyStore.trust(draft.hostKeyID, openSSH: k.openSSH) }
                    unknownKey = nil
                    Task { await fetchToken() }
                }
                Button("Cancel", role: .cancel) { unknownKey = nil }
            } message: {
                Text("First connection to \(profile.host).\n\nHost key fingerprint:\n\(unknownKey?.fingerprint ?? "")")
            }
            .fileImporter(isPresented: $importing, allowedContentTypes: [.data, .text, .plainText]) { result in
                guard case .success(let url) = result else { return }
                let scoped = url.startAccessingSecurityScopedResource()
                defer { if scoped { url.stopAccessingSecurityScopedResource() } }
                if let text = try? String(contentsOf: url, encoding: .utf8) { secrets.privateKey = text; message = nil }
                else { show("Couldn't read that file as text.", error: true) }
            }
        }
    }

    @ViewBuilder private var keySection: some View {
        if secrets.privateKey.isEmpty {
            Button { secrets.privateKey = SSHKeys.generate(); show("Key created. Add the public key below to ~/.ssh/authorized_keys on the server.", error: false) } label: {
                Label("Generate a new key", systemImage: "key.fill")
            }
            Button { importing = true } label: { Label("Import key file…", systemImage: "square.and.arrow.down") }
            Button {
                if let s = UIPasteboard.general.string, !s.isEmpty { secrets.privateKey = s } else { show("The clipboard is empty.", error: true) }
            } label: { Label("Paste private key", systemImage: "doc.on.clipboard") }
        } else {
            if let line = publicLine {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Public key — add to the server's ~/.ssh/authorized_keys").font(.caption).foregroundStyle(.secondary)
                    Text(line).font(.system(.caption2, design: .monospaced)).textSelection(.enabled)
                    HStack { CopyButton(text: line); ShareLink(item: line) { Label("Share", systemImage: "square.and.arrow.up").font(.caption) }.buttonStyle(.bordered).controlSize(.small) }
                }
            } else {
                Text("Private key saved in the Keychain.").font(.footnote).foregroundStyle(.secondary)
            }
            if !secrets.privateKey.hasPrefix(SSHKeys.rawPrefix) {
                SecureField("Key passphrase (if encrypted)", text: $secrets.passphrase)
            }
            Button("Remove key", role: .destructive) { secrets.privateKey = ""; secrets.passphrase = "" }
        }
    }

    private var canReach: Bool {
        draft.isComplete && (profile.auth == .password ? !secrets.password.isEmpty : !secrets.privateKey.isEmpty)
    }
    private var canSave: Bool { draft.isComplete }

    /// The profile with the text-field ports applied.
    private var draft: ServerProfile {
        var p = profile
        p.sshPort = Int(portText) ?? 0
        p.agentPort = Int(agentPortText) ?? 0
        return p
    }

    private func save() {
        profiles.upsert(draft, secrets: secrets)
        dismiss()
    }

    private func fetchToken() async {
        busy = true; message = nil
        defer { busy = false }
        do {
            secrets.agentToken = try await connection.readAgentToken(profile: draft, secrets: secrets)
            // Persist right away for an already-saved server; forgetting to tap Save left the old token in use.
            if profiles.profiles.contains(where: { $0.id == profile.id }) { profiles.upsert(draft, secrets: secrets) }
            show("Token read from the server (ends …\(secrets.agentToken.suffix(4)), \(secrets.agentToken.count) chars). \(profiles.profiles.contains(where: { $0.id == profile.id }) ? "Saved." : "Tap Save to keep it.")", error: false)
        } catch let issue as HostKeyIssue {
            if case .unknown(let openSSH, let fp) = issue {
                unknownKey = (fp, openSSH)
            } else { show(issue.localizedDescription, error: true) }
        } catch {
            show(ConnectionManager.describe(error, profile: draft), error: true)
        }
    }

    private func show(_ text: String, error: Bool) { message = text; isError = error }
}
