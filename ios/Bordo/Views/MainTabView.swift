import SwiftUI

/// Lets any screen hand the agent a task ("change the code", ...) and jump to the chat.
@MainActor
final class Router: ObservableObject {
    enum Tab: Hashable { case chat, apps, regions, observe, settings }
    @Published var tab: Tab = .chat
    @Published var pendingPrompt: String?

    func ask(_ text: String) { pendingPrompt = text; tab = .chat }
}

struct MainTabView: View {
    let client: AgentClient
    @StateObject private var router = Router()

    var body: some View {
        TabView(selection: $router.tab) {
            ChatTab(client: client)
                .tabItem { Label("Chat", systemImage: "sparkles") }.tag(Router.Tab.chat)
            AppsView(client: client)
                .tabItem { Label("Apps", systemImage: "shippingbox") }.tag(Router.Tab.apps)
            RegionsView(client: client)
                .tabItem { Label("Regions", systemImage: "globe") }.tag(Router.Tab.regions)
            ObserveView(client: client)
                .tabItem { Label("Observe", systemImage: "chart.xyaxis.line") }.tag(Router.Tab.observe)
            SettingsView()
                .tabItem { Label("Server", systemImage: "gearshape") }.tag(Router.Tab.settings)
        }
        .environmentObject(router)
    }
}

struct SettingsView: View {
    @EnvironmentObject var connection: ConnectionManager

    var body: some View {
        NavigationStack {
            List {
                if let p = connection.profile {
                    Section("Connected") {
                        LabeledContent("Server", value: p.displayName)
                        LabeledContent("SSH", value: "\(p.username)@\(p.host):\(p.sshPort)")
                        LabeledContent("Agent", value: "127.0.0.1:\(p.agentPort) via tunnel")
                        if let fp = HostKeyStore.trusted(p.hostKeyID) {
                            LabeledContent("Host key") { Text(HostKeyStore.fingerprint(openSSH: fp)).font(.caption2).textSelection(.enabled) }
                        }
                    }
                }
                Section {
                    Button("Disconnect", role: .destructive) { Task { await connection.disconnect() } }
                }
            }
            .navigationTitle("Server")
        }
    }
}
