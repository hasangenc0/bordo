import SwiftUI

struct ChatTab: View {
    let client: AgentClient
    @EnvironmentObject var router: Router
    @StateObject private var chats: Loader<[ChatSession]>
    @StateObject private var vm: ChatViewModel
    @State private var path: [String] = []
    @State private var renaming: ChatSession?
    @State private var newTitle = ""

    init(client: AgentClient) {
        self.client = client
        _chats = StateObject(wrappedValue: Loader { try await client.listChats() })
        _vm = StateObject(wrappedValue: ChatViewModel(client: client))
    }

    var body: some View {
        NavigationStack(path: $path) {
            List {
                if let e = chats.error { ErrorBanner(message: e).listRowInsets(.init()) }
                ForEach(chats.value ?? []) { c in
                    NavigationLink(value: c.id) {
                        HStack(spacing: 12) {
                            Image(systemName: "bubble.left.fill").foregroundStyle(Theme.gradient).font(.title3)
                            VStack(alignment: .leading, spacing: 3) {
                                Text(c.displayTitle).font(.body.weight(.medium)).lineLimit(1)
                                Text(Dates.relative(c.updatedAt)).font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer(minLength: 0)
                        }
                    }
                    .buttonStyle(.plain)
                    .cardRow()
                    .swipeActions {
                        Button(role: .destructive) { Task { try? await client.deleteChat(c.id); await chats.reload() } } label: { Label("Delete", systemImage: "trash") }
                        Button { renaming = c; newTitle = c.title } label: { Label("Rename", systemImage: "pencil") }.tint(.blue)
                    }
                }
            }
            .listStyle(.plain).screenBackground()
            .overlay { if (chats.value ?? []).isEmpty && chats.error == nil && !chats.loading { EmptyStateView(icon: "bubble.left.and.bubble.right", title: "No chats yet", hint: "Start one with the compose button.") } }
            .navigationTitle("Chat")
            .toolbar {
                ToolbarItem(placement: .primaryAction) { Button { path = [""] } label: { Image(systemName: "square.and.pencil") } }
            }
            .navigationDestination(for: String.self) { id in
                ChatView(vm: vm, chatID: id.isEmpty ? nil : id, title: chats.value?.first { $0.id == id }?.displayTitle ?? "New chat")
            }
            .refreshable { await chats.reload() }
            .task { await chats.poll(every: 15) }
            .alert("Rename chat", isPresented: Binding(get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
                TextField("Title", text: $newTitle)
                Button("Save") {
                    if let c = renaming, !newTitle.trimmed.isEmpty { Task { try? await client.renameChat(c.id, title: newTitle.trimmed); await chats.reload() } }
                    renaming = nil
                }
                Button("Cancel", role: .cancel) { renaming = nil }
            }
        }
        .onAppear { vm.onTurnDone = { Task { await chats.reload() } } }
        .onChange(of: router.pendingPrompt) { _, prompt in
            guard let prompt else { return }
            router.pendingPrompt = nil
            path = [""]
            Task { try? await Task.sleep(for: .milliseconds(400)); await vm.send(prompt) }
        }
    }
}

struct ChatView: View {
    @ObservedObject var vm: ChatViewModel
    let chatID: String?
    let title: String
    @Environment(\.scenePhase) private var scenePhase

    private let suggestions: [(String, String)] = [
        ("shippingbox", "What apps do I have running?"),
        ("plus.app", "Create a java web service called hello-api"),
        ("chart.xyaxis.line", "Show me CPU and memory for my apps"),
        ("arrow.triangle.pull", "Add a /health endpoint to my app and open a PR"),
    ]

    var body: some View {
        VStack(spacing: 0) {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 12) {
                        if vm.messages.isEmpty { welcome }
                        ForEach(vm.messages) { m in MessageRow(message: m).equatable().id(m.id) }
                        ForEach(vm.actions.filter { $0.status == .pending }) { a in ApprovalCard(action: a, vm: vm) }
                        if vm.busy { HStack(spacing: 6) { ProgressView().controlSize(.small); Text("Working…").font(.footnote).foregroundStyle(.secondary) }.padding(.leading, 2).id("busy") }
                    }
                    .padding(.horizontal, 16).padding(.vertical, 12)
                }
                .scrollDismissesKeyboard(.interactively)
                .onChange(of: vm.messages.last?.id) { _, _ in withAnimation { proxy.scrollTo(vm.busy ? "busy" : vm.messages.last?.id, anchor: .bottom) } }
                .onChange(of: vm.messages.last?.content) { _, _ in proxy.scrollTo(vm.messages.last?.id, anchor: .bottom) }
            }
            Composer(busy: vm.busy) { text in Task { await vm.send(text) } }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) {
                VStack(spacing: 0) {
                    Text(title).font(.headline).lineLimit(1)
                    if chatID != nil { Text(vm.connected ? "Connected" : "Connecting…").font(.caption2).foregroundStyle(vm.connected ? .green : .orange) }
                }
            }
        }
        .onAppear { vm.open(chatID) }
        .onDisappear { vm.close() }
        .onChange(of: scenePhase) { _, p in if p == .active { vm.resume() } }
    }

    private var welcome: some View {
        VStack(spacing: 14) {
            Image(systemName: "sparkles")
                .font(.system(size: 30, weight: .semibold)).foregroundStyle(.white)
                .frame(width: 68, height: 68)
                .background(Theme.gradient, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
                .shadow(color: Theme.accent.opacity(0.35), radius: 16, y: 8)
            Text("What do you want to build?").font(.title2.bold()).multilineTextAlignment(.center)
            Text("Create apps, ship them to your regions, check metrics, or change code.")
                .font(.subheadline).foregroundStyle(.secondary).multilineTextAlignment(.center)
                .padding(.bottom, 6)
            ForEach(suggestions, id: \.1) { icon, text in
                Button { Task { await vm.send(text) } } label: {
                    HStack(spacing: 12) {
                        Image(systemName: icon).foregroundStyle(Theme.accent).frame(width: 24)
                        Text(text).font(.subheadline).foregroundStyle(.primary).multilineTextAlignment(.leading)
                        Spacer(minLength: 0)
                    }
                    .card(padding: 14)
                }.buttonStyle(.plain)
            }
        }
        .padding(.top, 32)
    }
}

/// Owns the draft text. Typing only re-renders this view; the message list above never sees it.
struct Composer: View {
    let busy: Bool
    let onSend: (String) -> Void
    @State private var draft = ""

    private var canSend: Bool { !draft.trimmed.isEmpty && !busy }

    var body: some View {
        HStack(alignment: .bottom, spacing: 8) {
            GrowingTextView(text: $draft, placeholder: "Ask Bordo…")
                .padding(.leading, 16)
            Button {
                let t = draft
                draft = ""
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                onSend(t)
            } label: {
                Image(systemName: busy ? "ellipsis" : "arrow.up")
                    .font(.system(size: 15, weight: .bold)).foregroundStyle(.white)
                    .frame(width: 32, height: 32)
                    .background(canSend ? AnyShapeStyle(Theme.gradient) : AnyShapeStyle(Color(.systemGray3)), in: Circle())
            }
            .disabled(!canSend)
            .padding(.trailing, 6).padding(.bottom, 6)
        }
        .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 24, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 24, style: .continuous).stroke(Color(.separator).opacity(0.5), lineWidth: 0.5))
        .padding(.horizontal, 12).padding(.vertical, 8)
        .background(Color(.systemBackground))
    }
}

struct MessageRow: View, Equatable {
    let message: ChatMessage

    var body: some View {
        if let tool = message.tool {
            ToolCard(tool: tool)
        } else if message.role == .user {
            HStack {
                Spacer(minLength: 48)
                Text(message.content)
                    .textSelection(.enabled)
                    .padding(.horizontal, 14).padding(.vertical, 9)
                    .background(Theme.gradient, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
                    .foregroundStyle(.white)
            }
        } else if message.isError {
            ErrorBanner(message: message.content)
        } else {
            MarkdownText(text: message.content)
        }
    }
}

struct ApprovalCard: View {
    let action: ActionEvent
    @ObservedObject var vm: ChatViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Approval needed", systemImage: "hand.raised.fill").font(.subheadline.bold()).foregroundStyle(.orange)
            Text("\(action.kind.capitalized) \(action.target)").font(.body)
            HStack {
                Button("Approve") { Task { await vm.decide(action, approve: true) } }.buttonStyle(.borderedProminent)
                Button("Reject", role: .destructive) { Task { await vm.decide(action, approve: false) } }.buttonStyle(.bordered)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
    }
}
