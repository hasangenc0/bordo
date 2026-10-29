import Foundation
import Citadel
import Crypto
import NIO
import NIOSSH

enum SSHKeyError: LocalizedError {
    case unreadable
    case unsupported(String)
    case needsPassphrase
    case missingCredentials

    var errorDescription: String? {
        switch self {
        case .unreadable: return "That doesn't look like an OpenSSH private key."
        case .unsupported(let t): return "\(t) keys aren't supported yet. Use an ed25519 or RSA key, or generate one in the app."
        case .needsPassphrase: return "This key is encrypted. Enter its passphrase."
        case .missingCredentials: return "Add an SSH key or password for this server."
        }
    }
}

enum SSHKeys {
    /// Keys generated in-app are stored as the raw 32-byte seed; imported keys keep their OpenSSH text.
    static let rawPrefix = "bordo-ed25519:"

    static func generate() -> String {
        rawPrefix + Curve25519.Signing.PrivateKey().rawRepresentation.base64EncodedString()
    }

    /// The `authorized_keys` line for a stored key, when it can be derived without a passphrase.
    static func publicLine(stored: String, passphrase: String = "", comment: String = "bordo-ios") -> String? {
        guard let key = try? ed25519(stored: stored, passphrase: passphrase) else { return nil }
        var blob = Data()
        for part in [Data("ssh-ed25519".utf8), key.publicKey.rawRepresentation] {
            var len = UInt32(part.count).bigEndian
            blob.append(Data(bytes: &len, count: 4))
            blob.append(part)
        }
        return "ssh-ed25519 \(blob.base64EncodedString()) \(comment)"
    }

    private static func ed25519(stored: String, passphrase: String) throws -> Curve25519.Signing.PrivateKey {
        let text = stored.trimmed
        if text.hasPrefix(rawPrefix) {
            guard let raw = Data(base64Encoded: String(text.dropFirst(rawPrefix.count))) else { throw SSHKeyError.unreadable }
            return try Curve25519.Signing.PrivateKey(rawRepresentation: raw)
        }
        return try Curve25519.Signing.PrivateKey(sshEd25519: text + "\n", decryptionKey: passphrase.isEmpty ? nil : Data(passphrase.utf8))
    }

    static func authMethod(profile: ServerProfile, secrets: ServerSecrets) throws -> SSHAuthenticationMethod {
        switch profile.auth {
        case .password:
            guard !secrets.password.isEmpty else { throw SSHKeyError.missingCredentials }
            return .passwordBased(username: profile.username, password: secrets.password)
        case .key:
            let text = secrets.privateKey.trimmed
            guard !text.isEmpty else { throw SSHKeyError.missingCredentials }
            let dec = secrets.passphrase.isEmpty ? nil : Data(secrets.passphrase.utf8)
            if text.hasPrefix(rawPrefix) {
                return .ed25519(username: profile.username, privateKey: try ed25519(stored: text, passphrase: ""))
            }
            let type: SSHKeyType
            do { type = try SSHKeyDetection.detectPrivateKeyType(from: text) } catch { throw SSHKeyError.unreadable }
            do {
                switch type {
                case .ed25519:
                    return .ed25519(username: profile.username, privateKey: try Curve25519.Signing.PrivateKey(sshEd25519: text + "\n", decryptionKey: dec))
                case .rsa:
                    return .rsa(username: profile.username, privateKey: try Insecure.RSA.PrivateKey(sshRsa: text + "\n", decryptionKey: dec))
                default:
                    throw SSHKeyError.unsupported(type.description)
                }
            } catch let e as SSHKeyError {
                throw e
            } catch {
                // A wrong or missing passphrase surfaces as a generic parse/decrypt failure.
                throw dec == nil && text.contains("ENCRYPTED") || text.contains("bcrypt") ? SSHKeyError.needsPassphrase : error
            }
        }
    }
}
