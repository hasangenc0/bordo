import Foundation

/// A frame from the agent's `/ws/chat` socket (see agent/internal/server/types.go).
struct ChatFrame {
    enum Kind: String { case message, tool_call, tool_result, done, error, history, action }
    let kind: Kind
    let json: JSON

    var content: String { json["content"].str }
    var toolName: String { json["tool_name"].str }
}

/// One WebSocket to the agent for one chat. Frames are delivered in order on `frames`;
/// the stream finishes when the socket closes.
final class ChatSocket: @unchecked Sendable {
    private let task: URLSessionWebSocketTask
    let frames: AsyncStream<ChatFrame>

    init(client: AgentClient, chatID: String) {
        var comps = URLComponents(string: client.baseURL.absoluteString.replacingOccurrences(of: "http://", with: "ws://") + "/ws/chat")!
        comps.queryItems = [URLQueryItem(name: "chat_id", value: chatID)]
        if !client.token.isEmpty { comps.queryItems?.append(URLQueryItem(name: "token", value: client.token)) }
        let cfg = URLSessionConfiguration.ephemeral
        cfg.connectionProxyDictionary = [:]
        let t = URLSession(configuration: cfg).webSocketTask(with: comps.url!)
        task = t
        frames = AsyncStream { cont in
            func loop() {
                t.receive { result in
                    switch result {
                    case .failure:
                        cont.finish()
                    case .success(let msg):
                        let data: Data?
                        switch msg {
                        case .string(let s): data = Data(s.utf8)
                        case .data(let d): data = d
                        @unknown default: data = nil
                        }
                        if let data, let json = JSON.parse(data), let kind = ChatFrame.Kind(rawValue: json["type"].str) {
                            cont.yield(ChatFrame(kind: kind, json: json))
                        }
                        loop()
                    }
                }
            }
            loop()
            cont.onTermination = { _ in t.cancel(with: .goingAway, reason: nil) }
        }
        t.resume()
    }

    func send(_ text: String) async throws {
        let payload: JSON = ["type": "message", "content": .string(text)]
        let data = try JSONEncoder().encode(payload)
        try await task.send(.string(String(decoding: data, as: UTF8.self)))
    }

    func close() { task.cancel(with: .goingAway, reason: nil) }
}
