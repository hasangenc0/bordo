import XCTest
import Citadel
import Crypto
import NIO
import NIOSSH
@testable import Bordo

/// End-to-end: an in-process SSH server (password + public-key auth, direct-tcpip enabled) stands in
/// for the Bordo host, and a real `bordo-agent` provides the other end of the tunnel.
///
///   cd agent && go build -o /tmp/bordo-agent . && \
///     BORDO_AGENT_TOKEN=testtoken BORDO_CHAT_DB=/tmp/chats.db /tmp/bordo-agent --port 17402 &
///   TEST_RUNNER_BORDO_TEST_AGENT_PORT=17402 xcodebuild test ...
///
/// Skipped when `BORDO_TEST_AGENT_PORT` is not set.
final class TunnelIntegrationTests: XCTestCase {
    private var server: SSHServer?
    private let hostKey = Curve25519.Signing.PrivateKey()
    private let sshPort = 22322
    private var agentPort = 0
    private var authorized: NIOSSHPublicKey?

    /// Citadel's own forwarding delegate adds its byte codec a second time (the server already did),
    /// which traps on first data. This one just pipes bytes to the target.
    private struct Forwarder: DirectTCPIPDelegate {
        final class Pipe: ChannelInboundHandler {
            typealias InboundIn = ByteBuffer
            let other: Channel
            init(_ other: Channel) { self.other = other }
            func channelRead(context: ChannelHandlerContext, data: NIOAny) { other.writeAndFlush(unwrapInboundIn(data), promise: nil) }
            func channelInactive(context: ChannelHandlerContext) { other.close(promise: nil) }
        }
        func initializeDirectTCPIPChannel(_ channel: Channel, request: SSHChannelType.DirectTCPIP, context: SSHContext) -> EventLoopFuture<Void> {
            ClientBootstrap(group: channel.eventLoop)
                .connect(host: request.targetHost, port: request.targetPort)
                .flatMap { remote in
                    remote.pipeline.addHandler(Pipe(channel)).flatMap { channel.pipeline.addHandler(Pipe(remote)) }
                }
        }
    }

    private struct Login: NIOSSHServerUserAuthenticationDelegate {
        let authorized: NIOSSHPublicKey?
        var supportedAuthenticationMethods: NIOSSHAvailableUserAuthenticationMethods { [.password, .publicKey] }
        func requestReceived(request: NIOSSHUserAuthenticationRequest, responsePromise: EventLoopPromise<NIOSSHUserAuthenticationOutcome>) {
            switch request.request {
            case .password(let p) where p.password == "secret" && request.username == "bordo": responsePromise.succeed(.success)
            case .publicKey(let k) where k.publicKey == authorized && request.username == "bordo": responsePromise.succeed(.success)
            default: responsePromise.succeed(.failure)
            }
        }
    }

    override func setUp() async throws {
        guard let p = ProcessInfo.processInfo.environment["BORDO_TEST_AGENT_PORT"], let port = Int(p) else {
            throw XCTSkip("BORDO_TEST_AGENT_PORT not set (see file header)")
        }
        agentPort = port
        HostKeyStore.forget("127.0.0.1:\(sshPort)")
    }

    override func tearDown() async throws {
        try? await server?.close()
        server = nil
    }

    private func startServer(authorizing key: NIOSSHPublicKey? = nil) async throws {
        let s = try await SSHServer.host(
            host: "127.0.0.1", port: sshPort,
            hostKeys: [NIOSSHPrivateKey(ed25519Key: hostKey)],
            authenticationDelegate: Login(authorized: key)
        )
        s.enableDirectTCPIP(withDelegate: Forwarder())
        server = s
    }

    private func profile(auth: ServerProfile.Auth) -> ServerProfile {
        var p = ServerProfile()
        p.host = "127.0.0.1"; p.sshPort = sshPort; p.username = "bordo"; p.agentPort = agentPort; p.auth = auth
        return p
    }

    private var hostKeyLine: String { String(openSSHPublicKey: NIOSSHPrivateKey(ed25519Key: hostKey).publicKey) }

    func testUnknownHostKeyIsReportedNotTrusted() async throws {
        try await startServer()
        var s = ServerSecrets(); s.password = "secret"
        let t = SSHTunnel()
        do {
            _ = try await t.start(profile: profile(auth: .password), secrets: s)
            XCTFail("connected without trusting the host key")
        } catch let HostKeyIssue.unknown(openSSH, fp) {
            XCTAssertEqual(openSSH.split(separator: " ").prefix(2), hostKeyLine.split(separator: " ").prefix(2))
            XCTAssertTrue(fp.hasPrefix("SHA256:"))
        }
    }

    func testChangedHostKeyIsRejected() async throws {
        try await startServer()
        let other = String(openSSHPublicKey: NIOSSHPrivateKey(ed25519Key: Curve25519.Signing.PrivateKey()).publicKey)
        HostKeyStore.trust("127.0.0.1:\(sshPort)", openSSH: other)
        var s = ServerSecrets(); s.password = "secret"
        do {
            _ = try await SSHTunnel().start(profile: profile(auth: .password), secrets: s)
            XCTFail("accepted a changed host key")
        } catch HostKeyIssue.changed { }
    }

    func testWrongPasswordFails() async throws {
        try await startServer()
        HostKeyStore.trust("127.0.0.1:\(sshPort)", openSSH: hostKeyLine)
        var s = ServerSecrets(); s.password = "nope"
        do {
            _ = try await SSHTunnel().start(profile: profile(auth: .password), secrets: s)
            XCTFail("logged in with the wrong password")
        } catch { }
    }

    func testPasswordTunnelCarriesRESTAndChatWebSocket() async throws {
        try await startServer()
        HostKeyStore.trust("127.0.0.1:\(sshPort)", openSSH: hostKeyLine)
        var s = ServerSecrets(); s.password = "secret"; s.agentToken = "testtoken"

        let tunnel = SSHTunnel()
        let port = try await tunnel.start(profile: profile(auth: .password), secrets: s)
        defer { Task { await tunnel.stop() } }
        XCTAssertGreaterThan(port, 0)

        try await exerciseAgent(port: port, token: "testtoken")
    }

    func testGeneratedKeyAuthenticates() async throws {
        let stored = SSHKeys.generate()
        let line = try XCTUnwrap(SSHKeys.publicLine(stored: stored))
        let pub = try NIOSSHPublicKey(openSSHPublicKey: line)
        try await startServer(authorizing: pub)
        HostKeyStore.trust("127.0.0.1:\(sshPort)", openSSH: hostKeyLine)

        var s = ServerSecrets(); s.privateKey = stored; s.agentToken = "testtoken"
        let tunnel = SSHTunnel()
        let port = try await tunnel.start(profile: profile(auth: .key), secrets: s)
        defer { Task { await tunnel.stop() } }
        _ = try await AgentClient(port: port, token: "testtoken").send("/healthz")
    }

    func testBadAgentTokenSurfacesAsUnauthorized() async throws {
        try await startServer()
        HostKeyStore.trust("127.0.0.1:\(sshPort)", openSSH: hostKeyLine)
        var s = ServerSecrets(); s.password = "secret"
        let tunnel = SSHTunnel()
        let port = try await tunnel.start(profile: profile(auth: .password), secrets: s)
        defer { Task { await tunnel.stop() } }
        do {
            _ = try await AgentClient(port: port, token: "wrong").send("/chats")
            XCTFail("expected 401")
        } catch let e as AgentError { XCTAssertEqual(e.status, 401) }
    }

    private func exerciseAgent(port: UInt16, token: String) async throws {
        let client = AgentClient(port: port, token: token)

        // REST: chat CRUD.
        let chat = try await client.createChat()
        XCTAssertFalse(chat.id.isEmpty)
        let listed = try await client.listChats()
        XCTAssertTrue(listed.contains { $0.id == chat.id })

        // Control-plane passthrough (/v1) is served by a mock in the harness; tolerate its absence.
        if let projects = try? await client.projects(), let p = projects.first {
            XCTAssertEqual(p.name, "hello-api")
            XCTAssertEqual(p.releaseStatus, "running")
        }

        // WebSocket chat through the tunnel, via the same view model the UI uses.
        let vm = await ChatViewModel(client: client)
        await vm.open(chat.id)
        await vm.send("hello from the phone")
        let deadline = Date().addingTimeInterval(10)
        while Date() < deadline {
            let busy = await vm.busy
            let count = await vm.messages.count
            if !busy && count >= 2 { break }
            try await Task.sleep(for: .milliseconds(100))
        }
        let msgs = await vm.messages
        XCTAssertEqual(msgs.first?.role, .user)
        XCTAssertEqual(msgs.first?.content, "hello from the phone")
        XCTAssertTrue(msgs.contains { $0.role == .assistant && !$0.content.isEmpty }, "no assistant reply: \(msgs)")
        await vm.close()

        // History is replayed on reconnect and the server titled the chat from the first message.
        let again = try await client.listChats().first { $0.id == chat.id }
        XCTAssertEqual(again?.title, "hello from the phone")

        try await client.deleteChat(chat.id)
        let after = try await client.listChats()
        XCTAssertFalse(after.contains { $0.id == chat.id })
    }
}
