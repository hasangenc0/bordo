import SwiftUI

@main
struct BordoApp: App {
    @StateObject private var profiles = ProfileStore()
    @StateObject private var connection = ConnectionManager()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(profiles)
                .environmentObject(connection)
                .tint(Theme.accent)
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active { Task { await connection.resumeIfNeeded() } }
                }
        }
    }
}

struct RootView: View {
    @EnvironmentObject var connection: ConnectionManager

    var body: some View {
        Group {
            if connection.state == .connected, let client = connection.client {
                MainTabView(client: client)
                    .id(ObjectIdentifier(client)) // a new tunnel means fresh view models
            } else {
                ServersView()
            }
        }
        .alert(item: $connection.trustRequest) { req in
            Alert(
                title: Text("Trust this server?"),
                message: Text("First connection to \(req.profile.host).\n\nHost key fingerprint:\n\(req.fingerprint)\n\nCompare it with `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub` on the server."),
                primaryButton: .default(Text("Trust and connect")) { Task { await connection.trust(req) } },
                secondaryButton: .cancel()
            )
        }
    }
}

enum Theme {
    /// Bordeaux red, after the name.
    static let accent = Color(red: 0.55, green: 0.12, blue: 0.24)
    static let accentLight = Color(red: 0.80, green: 0.27, blue: 0.40)
    static let gradient = LinearGradient(colors: [accentLight, accent], startPoint: .topLeading, endPoint: .bottomTrailing)
    static let radius: CGFloat = 16
    static let background = Color(.systemGroupedBackground)
    static let card = Color(.secondarySystemGroupedBackground)
}

extension View {
    /// Rounded surface used for list rows and panels.
    func card(padding: CGFloat = 14) -> some View {
        self.padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
    }

    /// A List row that looks like a floating card.
    func cardRow() -> some View {
        self.padding(14)
            .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .listRowInsets(EdgeInsets(top: 5, leading: 16, bottom: 5, trailing: 16))
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
    }

    func screenBackground() -> some View {
        self.scrollContentBackground(.hidden).background(Theme.background)
    }
}
