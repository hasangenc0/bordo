import Foundation

/// A Bordo host reachable over SSH. Non-secret fields are persisted as JSON in
/// UserDefaults; secrets are in the Keychain under `<id>.<kind>`.
struct ServerProfile: Codable, Identifiable, Hashable {
    enum Auth: String, Codable, CaseIterable, Identifiable {
        case key = "SSH key"
        case password = "Password"
        var id: String { rawValue }
    }

    var id = UUID()
    var name = ""
    var host = ""
    var sshPort = 22
    var username = ""
    var auth: Auth = .key
    /// Port the agent listens on, on the *remote* machine (loopback side of the tunnel).
    var agentPort = 7402
    /// Where Bordo is installed on the server; used to read the agent token from its `.env`.
    var bordoDir = "/opt/bordo"

    var displayName: String { name.isEmpty ? host : name }
    var isComplete: Bool { !host.trimmed.isEmpty && !username.trimmed.isEmpty && (1...65535).contains(sshPort) && (1...65535).contains(agentPort) }

    /// Host-key trust is keyed by this, so changing the port is a different trust entry.
    var hostKeyID: String { "\(host.trimmed.lowercased()):\(sshPort)" }
}

/// Secrets for a profile, loaded from / saved to the Keychain.
struct ServerSecrets: Equatable {
    /// OpenSSH private key text (`-----BEGIN OPENSSH PRIVATE KEY-----`), or a `bordo-ed25519:<base64>` raw key made in-app.
    var privateKey = ""
    var passphrase = ""
    var password = ""
    var agentToken = ""

    static func load(_ id: UUID) -> ServerSecrets {
        ServerSecrets(
            privateKey: Keychain.get(account: "\(id).key") ?? "",
            passphrase: Keychain.get(account: "\(id).passphrase") ?? "",
            password: Keychain.get(account: "\(id).password") ?? "",
            agentToken: Keychain.get(account: "\(id).token") ?? ""
        )
    }

    func save(_ id: UUID) {
        Keychain.set(privateKey, account: "\(id).key")
        Keychain.set(passphrase, account: "\(id).passphrase")
        Keychain.set(password, account: "\(id).password")
        Keychain.set(agentToken, account: "\(id).token")
    }

    static func delete(_ id: UUID) {
        for k in ["key", "passphrase", "password", "token"] { Keychain.set(nil, account: "\(id).\(k)") }
    }
}

@MainActor
final class ProfileStore: ObservableObject {
    @Published private(set) var profiles: [ServerProfile] = []
    private let key = "bordo.profiles"

    init() {
        if let d = UserDefaults.standard.data(forKey: key),
           let p = try? JSONDecoder().decode([ServerProfile].self, from: d) { profiles = p }
    }

    func upsert(_ p: ServerProfile, secrets: ServerSecrets) {
        if let i = profiles.firstIndex(where: { $0.id == p.id }) { profiles[i] = p } else { profiles.append(p) }
        secrets.save(p.id)
        persist()
    }

    func delete(_ p: ServerProfile) {
        profiles.removeAll { $0.id == p.id }
        ServerSecrets.delete(p.id)
        HostKeyStore.forget(p.hostKeyID)
        persist()
    }

    private func persist() {
        if let d = try? JSONEncoder().encode(profiles) { UserDefaults.standard.set(d, forKey: key) }
    }
}

extension String {
    var trimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}
