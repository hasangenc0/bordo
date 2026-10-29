import Foundation

struct AgentError: LocalizedError {
    let status: Int
    let message: String
    var errorDescription: String? { message }
}

/// REST client for the agent (and, through its `/v1` passthrough, the control plane).
/// All traffic goes to the loopback end of the SSH tunnel.
final class AgentClient: @unchecked Sendable {
    let baseURL: URL
    let token: String
    private let session: URLSession

    init(port: UInt16, token: String) {
        baseURL = URL(string: "http://127.0.0.1:\(port)")!
        self.token = token
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 30
        cfg.waitsForConnectivity = false
        cfg.connectionProxyDictionary = [:] // never route loopback through a system proxy
        session = URLSession(configuration: cfg)
    }

    /// Authenticated request. `path` may include a query string.
    func request(_ path: String, method: String = "GET", body: JSON? = nil) -> URLRequest {
        var r = URLRequest(url: URL(string: baseURL.absoluteString + path)!)
        r.httpMethod = method
        if !token.isEmpty { r.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let body {
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try? JSONEncoder().encode(body)
        }
        return r
    }

    @discardableResult
    func send(_ path: String, method: String = "GET", body: JSON? = nil) async throws -> JSON {
        let (data, resp): (Data, URLResponse)
        do {
            (data, resp) = try await session.data(for: request(path, method: method, body: body))
        } catch let e as URLError where e.code == .cancelled {
            throw e
        } catch {
            throw AgentError(status: 0, message: "Can't reach the agent through the tunnel.")
        }
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        let json = JSON.parse(data) ?? .null
        guard (200..<300).contains(status) else {
            let msg = json["error"].string ?? (status == 401 ? "The agent rejected the agent token. Tap \"Read token from server\" again, or compare it with BORDO_AGENT_TOKEN in the agent container." : "Request failed (\(status))")
            throw AgentError(status: status, message: msg)
        }
        return json
    }

    // MARK: Chats

    func listChats() async throws -> [ChatSession] {
        try await send("/chats")["chats"].array.map(ChatSession.init)
    }

    func createChat() async throws -> ChatSession {
        ChatSession(json: try await send("/chats", method: "POST", body: [:]))
    }

    func deleteChat(_ id: String) async throws { try await send("/chats/\(id)", method: "DELETE") }

    func renameChat(_ id: String, title: String) async throws {
        try await send("/chats/\(id)/title", method: "PUT", body: ["title": .string(title)])
    }

    func decide(approval id: String, approve: Bool) async throws {
        try await send("/approvals/\(id)/\(approve ? "approve" : "reject")", method: "POST")
    }

    // MARK: Control plane (via the agent's /v1 passthrough)

    func cp(_ path: String, method: String = "GET", body: JSON? = nil) async throws -> JSON {
        try await send("/v1" + path, method: method, body: body)
    }

    func projects() async throws -> [Project] {
        try await cp("/projects?include=release_status")["projects"].array.map(Project.init)
    }

    func regions() async throws -> [Region] { try await cp("/regions")["regions"].array.map(Region.init) }

    func nodes(region: String) async throws -> [NodeHealth] {
        let enc = region.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? region
        return try await cp("/regions/\(enc)/health")["nodes"].array.map(NodeHealth.init)
    }

    func releases(project: String) async throws -> [Release] {
        try await cp("/releases?project_id=\(project)")["releases"].array.map(Release.init)
    }

    func builds(project: String) async throws -> [Build] {
        try await cp("/builds?project_id=\(project)")["builds"].array.map(Build.init)
    }

    func templates() async throws -> [Template] { try await cp("/templates")["templates"].array.map(Template.init) }
}
