import Foundation
import SwiftUI

/// Owns the SSH tunnel and the `AgentClient` that rides on it. Views observe `state`;
/// everything else reads `client` once connected.
@MainActor
final class ConnectionManager: ObservableObject {
    enum State: Equatable {
        case disconnected
        case connecting(String)
        case connected
        case failed(String)
    }

    /// A first-time host key that needs the user's explicit approval.
    struct TrustRequest: Identifiable {
        let id = UUID()
        let profile: ServerProfile
        let fingerprint: String
        let openSSH: String
    }

    @Published private(set) var state: State = .disconnected
    @Published private(set) var profile: ServerProfile?
    @Published private(set) var client: AgentClient?
    @Published var trustRequest: TrustRequest?

    private let tunnel = SSHTunnel()
    private var attempt = 0

    init() {
        tunnel.onDisconnect = { [weak self] in
            Task { @MainActor in self?.tunnelDropped() }
        }
    }

    func connect(_ profile: ServerProfile) async {
        attempt += 1
        let mine = attempt
        self.profile = profile
        client = nil
        state = .connecting("Opening SSH to \(profile.host)…")
        let secrets = ServerSecrets.load(profile.id)
        do {
            let port = try await tunnel.start(profile: profile, secrets: secrets)
            guard mine == attempt else { return }
            state = .connecting("Reaching the agent…")
            let c = AgentClient(port: port, token: secrets.agentToken)
            // /healthz is unauthenticated; /chats proves the token is accepted.
            try await c.send("/healthz")
            try await c.send("/chats")
            guard mine == attempt else { return }
            client = c
            state = .connected
        } catch {
            guard mine == attempt else { return }
            await tunnel.stop()
            if let issue = error as? HostKeyIssue, case .unknown(let openSSH, let fp) = issue {
                trustRequest = TrustRequest(profile: profile, fingerprint: fp, openSSH: openSSH)
                state = .disconnected
            } else if let e = error as? AgentError, e.status == 401 {
                let t = secrets.agentToken
                let sent = t.isEmpty ? "no token" : "a token ending …\(t.suffix(4)) (\(t.count) chars)"
                state = .failed("The agent rejected the token. The app sent \(sent). Compare it with the server's BORDO_AGENT_TOKEN, or read the token again in the server's settings.")
            } else {
                state = .failed(Self.describe(error, profile: profile))
            }
        }
    }

    func trust(_ req: TrustRequest) async {
        HostKeyStore.trust(req.profile.hostKeyID, openSSH: req.openSSH)
        trustRequest = nil
        await connect(req.profile)
    }

    func disconnect() async {
        attempt += 1
        client = nil
        state = .disconnected
        await tunnel.stop()
    }

    /// Reconnects if iOS suspended the app long enough for the SSH socket to die.
    func resumeIfNeeded() async {
        guard let profile, state == .connected || isFailedAfterConnect else { return }
        if !tunnel.isConnected { await connect(profile) }
    }

    /// Reads `BORDO_INTERNAL_TOKEN` from the server's deploy `.env` over the open SSH session.
    func readAgentToken(profile: ServerProfile, secrets: ServerSecrets) async throws -> String {
        let t = SSHTunnel()
        defer { Task { await t.stop() } }
        _ = try await t.start(profile: profile, secrets: secrets)
        let dir = profile.bordoDir.trimmed.replacingOccurrences(of: "'", with: "'\\''")
        // Never fail or write to stderr: Citadel turns both into opaque errors, so report on stdout instead.
        let script = """
        # What the agent is actually running with beats what the .env says now.
        for dk in "docker" "sudo -n docker"; do
          c=$($dk ps --filter name=bordo-agent --format '{{.Names}}' 2>/dev/null | head -n1)
          if [ -n "$c" ]; then
            v=$($dk exec "$c" printenv BORDO_AGENT_TOKEN 2>/dev/null | tr -d '\\r\\n')
            if [ -n "$v" ]; then echo "TOKEN:$v"; exit 0; fi
          fi
        done
        for d in '\(dir)' /opt/bordo "$HOME/bordo" "$HOME/.bordo"; do
          f="$d/deploy/bordo/.env"
          if [ -f "$f" ]; then
            if [ -r "$f" ]; then v=$(sed -n 's/^BORDO_INTERNAL_TOKEN=//p' "$f" | head -n1)
            else v=$(sudo -n sed -n 's/^BORDO_INTERNAL_TOKEN=//p' "$f" 2>/dev/null | head -n1); fi
            if [ -n "$v" ]; then echo "TOKEN:$v"; exit 0; fi
            echo "ERR:$f has no BORDO_INTERNAL_TOKEN, or this user can't read it (try the root or install user)"; exit 0
          fi
        done
        f=$(timeout 8 find "$HOME" /opt /srv /home /root /usr/local -maxdepth 6 -path '*/deploy/bordo/.env' 2>/dev/null | head -n1)
        if [ -n "$f" ]; then
          v=$(sed -n 's/^BORDO_INTERNAL_TOKEN=//p' "$f" 2>/dev/null | head -n1)
          [ -z "$v" ] && v=$(sudo -n sed -n 's/^BORDO_INTERNAL_TOKEN=//p' "$f" 2>/dev/null | head -n1)
          if [ -n "$v" ]; then echo "TOKEN:$v"; exit 0; fi
          echo "ERR:found $f but can't read a token from it (try the root or install user)"; exit 0
        fi
        echo "ERR:no deploy/bordo/.env found under '\(dir)', /opt/bordo or ~/bordo. Set the Bordo install directory under Advanced."
        """
        let out: String
        do { out = try await t.run(script) } catch {
            throw AgentError(status: 0, message: "Running a command on the server failed (\(error)). The SSH user may not be allowed to run commands.")
        }
        if let line = out.split(separator: "\n").first(where: { $0.hasPrefix("TOKEN:") }) {
            return String(line.dropFirst(6)).trimmed.trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
        }
        let err = out.split(separator: "\n").first(where: { $0.hasPrefix("ERR:") }).map { String($0.dropFirst(4)) }
        throw AgentError(status: 0, message: err ?? "Unexpected reply from the server: \(out.trimmed.prefix(200))")
    }

    private var isFailedAfterConnect: Bool {
        if case .failed = state { return client != nil }
        return false
    }

    private func tunnelDropped() {
        guard state == .connected else { return }
        state = .failed("The SSH connection was lost.")
    }

    static func describe(_ error: Error, profile: ServerProfile) -> String {
        if let e = error as? LocalizedError, let d = e.errorDescription { return d }
        let ns = error as NSError
        if ns.domain == NSPOSIXErrorDomain || ns.domain == NSURLErrorDomain {
            return "Couldn't reach \(profile.host):\(profile.sshPort). Check the address and that the phone has network access."
        }
        let text = String(describing: error)
        if text.localizedCaseInsensitiveContains("auth") {
            return "SSH login was rejected for \(profile.username)@\(profile.host). Check the key or password."
        }
        if text.localizedCaseInsensitiveContains("timeout") || text.localizedCaseInsensitiveContains("timed out") {
            return "Timed out connecting to \(profile.host):\(profile.sshPort)."
        }
        return error.localizedDescription
    }
}
