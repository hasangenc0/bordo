import Foundation
import Crypto
import NIO
import NIOSSH

/// Why a host key was rejected. The UI turns `.unknown` into a trust prompt and
/// `.changed` into a hard stop (possible man-in-the-middle).
enum HostKeyIssue: Error, Equatable, LocalizedError {
    case unknown(openSSH: String, fingerprint: String)
    case changed(fingerprint: String, expected: String)

    var errorDescription: String? {
        switch self {
        case .unknown(_, let fp): return "Unrecognized host key \(fp)."
        case .changed(let fp, let expected):
            return "The host key changed (now \(fp), was \(expected)). Someone may be intercepting the connection."
        }
    }
}

/// Trust-on-first-use host key pinning, like ~/.ssh/known_hosts. Public keys aren't secret,
/// so UserDefaults is fine.
enum HostKeyStore {
    private static let key = "bordo.hostkeys"

    private static func all() -> [String: String] {
        UserDefaults.standard.dictionary(forKey: key) as? [String: String] ?? [:]
    }

    static func trusted(_ id: String) -> String? { all()[id] }

    static func trust(_ id: String, openSSH: String) {
        var a = all(); a[id] = openSSH
        UserDefaults.standard.set(a, forKey: key)
    }

    static func forget(_ id: String) {
        var a = all(); a[id] = nil
        UserDefaults.standard.set(a, forKey: key)
    }

    /// `SHA256:<base64, unpadded>` of the key blob, as printed by `ssh-keygen -lf`.
    static func fingerprint(openSSH: String) -> String {
        let parts = openSSH.split(separator: " ")
        guard parts.count >= 2, let blob = Data(base64Encoded: String(parts[1])) else { return "unknown" }
        let b64 = Data(SHA256.hash(data: blob)).base64EncodedString().replacingOccurrences(of: "=", with: "")
        return "SHA256:\(b64)"
    }
}

/// Checks the server key against the pinned one. Records the outcome in `issue` because the
/// SSH stack wraps validator errors in its own types by the time they reach the caller.
final class PinnedHostKeyValidator: NIOSSHClientServerAuthenticationDelegate, @unchecked Sendable {
    private let id: String
    private let lock = NSLock()
    private var _issue: HostKeyIssue?
    var issue: HostKeyIssue? { lock.lock(); defer { lock.unlock() }; return _issue }

    init(id: String) { self.id = id }

    func validateHostKey(hostKey: NIOSSHPublicKey, validationCompletePromise: EventLoopPromise<Void>) {
        let presented = String(openSSHPublicKey: hostKey)
        guard let pinned = HostKeyStore.trusted(id) else {
            let err = HostKeyIssue.unknown(openSSH: presented, fingerprint: HostKeyStore.fingerprint(openSSH: presented))
            lock.lock(); _issue = err; lock.unlock()
            validationCompletePromise.fail(err)
            return
        }
        // Compare the "type base64" pair only; the optional trailing comment isn't part of the key.
        let a = presented.split(separator: " ").prefix(2), b = pinned.split(separator: " ").prefix(2)
        if a == b {
            validationCompletePromise.succeed(())
        } else {
            let err = HostKeyIssue.changed(fingerprint: HostKeyStore.fingerprint(openSSH: presented), expected: HostKeyStore.fingerprint(openSSH: pinned))
            lock.lock(); _issue = err; lock.unlock()
            validationCompletePromise.fail(err)
        }
    }
}
