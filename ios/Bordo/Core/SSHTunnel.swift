import Foundation
import Network
import Citadel
import NIO
import NIOCore
import NIOSSH

enum TunnelError: LocalizedError {
    case listener(String)
    case notConnected

    var errorDescription: String? {
        switch self {
        case .listener(let m): return "Couldn't start the local tunnel endpoint: \(m)"
        case .notConnected: return "The SSH connection is closed."
        }
    }
}

/// SSH local port forward. Opens one SSH session to the server and listens on a loopback port;
/// every connection accepted there is carried over an SSH `direct-tcpip` channel to the agent
/// (`127.0.0.1:<agentPort>` on the server). The rest of the app then talks plain HTTP/WebSocket
/// to `127.0.0.1:<port>` with URLSession and never has to know SSH is underneath. The agent port
/// stays closed to the internet; reaching it requires an SSH login.
final class SSHTunnel: @unchecked Sendable {
    private let queue = DispatchQueue(label: "io.bordo.tunnel")
    private var client: SSHClient?
    private var listener: NWListener?
    private var remotePort = 7402
    private(set) var localPort: UInt16 = 0

    /// Called (on an arbitrary thread) when the SSH session drops.
    var onDisconnect: (@Sendable () -> Void)?

    var isConnected: Bool { client?.isConnected ?? false }

    func start(profile: ServerProfile, secrets: ServerSecrets) async throws -> UInt16 {
        await stop()
        remotePort = profile.agentPort

        let validator = PinnedHostKeyValidator(id: profile.hostKeyID)
        let auth = try SSHKeys.authMethod(profile: profile, secrets: secrets)
        var settings = SSHClientSettings(
            host: profile.host.trimmed,
            port: profile.sshPort,
            authenticationMethod: { auth },
            hostKeyValidator: .custom(validator)
        )
        settings.connectTimeout = .seconds(15)

        let c: SSHClient
        do {
            c = try await SSHClient.connect(to: settings)
        } catch {
            if let issue = validator.issue { throw issue }
            throw error
        }
        c.onDisconnect { [weak self] in self?.onDisconnect?() }
        client = c

        do {
            localPort = try await startListener()
        } catch {
            await stop()
            throw error
        }
        return localPort
    }

    func stop() async {
        listener?.cancel()
        listener = nil
        localPort = 0
        let c = client
        client = nil
        try? await c?.close()
    }

    /// Runs a command on the server (used to read the agent token). Output is capped.
    func run(_ command: String) async throws -> String {
        guard let client else { throw TunnelError.notConnected }
        let buf = try await client.executeCommand(command, maxResponseSize: 64 * 1024)
        return String(buffer: buf)
    }

    // MARK: Loopback listener

    private func startListener() async throws -> UInt16 {
        let params = NWParameters.tcp
        // Loopback only: nothing else on the phone's network can reach the tunnel.
        params.requiredInterfaceType = .loopback
        let l: NWListener
        do { l = try NWListener(using: params) } catch { throw TunnelError.listener(error.localizedDescription) }
        listener = l

        l.newConnectionHandler = { [weak self] conn in self?.accept(conn) }
        return try await withCheckedThrowingContinuation { (cont: CheckedContinuation<UInt16, Error>) in
            var resumed = false
            l.stateUpdateHandler = { state in
                guard !resumed else { return }
                switch state {
                case .ready:
                    resumed = true
                    cont.resume(returning: l.port?.rawValue ?? 0)
                case .failed(let e):
                    resumed = true
                    cont.resume(throwing: TunnelError.listener(e.localizedDescription))
                default: break
                }
            }
            l.start(queue: queue)
        }
    }

    private func accept(_ conn: NWConnection) {
        guard let client else { conn.cancel(); return }
        conn.start(queue: queue)
        let port = remotePort
        Task { [queue] in
            do {
                let origin = try SocketAddress(ipAddress: "127.0.0.1", port: 0)
                let channel = try await client.createDirectTCPIPChannel(
                    using: SSHChannelType.DirectTCPIP(targetHost: "127.0.0.1", targetPort: port, originatorAddress: origin)
                ) { channel in
                    channel.pipeline.addHandler(ToConnection(conn))
                }
                Self.pump(from: conn, to: channel, queue: queue)
            } catch {
                conn.cancel()
            }
        }
    }

    /// Local socket -> SSH channel.
    private static func pump(from conn: NWConnection, to channel: Channel, queue: DispatchQueue) {
        conn.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { data, _, isComplete, error in
            if let data, !data.isEmpty {
                var buf = channel.allocator.buffer(capacity: data.count)
                buf.writeBytes(data)
                channel.writeAndFlush(buf, promise: nil)
            }
            if isComplete || error != nil {
                channel.close(promise: nil)
                conn.cancel()
                return
            }
            pump(from: conn, to: channel, queue: queue)
        }
    }

    /// SSH channel -> local socket.
    private final class ToConnection: ChannelInboundHandler {
        typealias InboundIn = ByteBuffer
        private let conn: NWConnection
        init(_ conn: NWConnection) { self.conn = conn }

        func channelRead(context: ChannelHandlerContext, data: NIOAny) {
            let buf = unwrapInboundIn(data)
            conn.send(content: Data(buf.readableBytesView), completion: .contentProcessed { _ in })
        }
        func channelInactive(context: ChannelHandlerContext) { conn.cancel() }
        func errorCaught(context: ChannelHandlerContext, error: Error) { context.close(promise: nil) }
    }
}
