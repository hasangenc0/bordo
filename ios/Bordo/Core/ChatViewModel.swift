import Foundation
import SwiftUI

/// One conversation: owns the WebSocket, turns frames into messages/tool cards, and
/// mirrors the web console's `useWebSocket` behavior (history replay, tool-call pairing,
/// queued first message, reconnect).
@MainActor
final class ChatViewModel: ObservableObject {
    @Published private(set) var messages: [ChatMessage] = []
    @Published private(set) var actions: [ActionEvent] = []
    @Published private(set) var connected = false
    @Published private(set) var busy = false
    @Published private(set) var chatID: String?

    private let client: AgentClient
    private var socket: ChatSocket?
    private var reader: Task<Void, Never>?
    private var reconnect: Task<Void, Never>?
    private var epoch = 0
    private var seq = 0
    private var queued: [String] = []

    /// Called after a turn finishes so the list can pick up the server-generated title.
    var onTurnDone: (() -> Void)?

    init(client: AgentClient) { self.client = client }

    private func nextID(_ p: String) -> String { seq += 1; return "\(p)-\(seq)" }

    func open(_ id: String?) {
        close()
        epoch += 1
        messages = []
        actions = []
        busy = false
        queued = []
        chatID = id
        if let id { connect(id, epoch: epoch) }
    }

    func close() {
        epoch += 1
        reconnect?.cancel()
        reader?.cancel()
        socket?.close()
        socket = nil
        connected = false
    }

    /// Re-establish after the app returns from the background.
    func resume() {
        guard let id = chatID, !connected else { return }
        connect(id, epoch: epoch)
    }

    func send(_ text: String) async {
        let t = text.trimmed
        guard !t.isEmpty, !busy else { return }
        if chatID == nil {
            // A brand-new conversation gets its chat (and socket) on the first message.
            guard let s = try? await client.createChat() else {
                messages.append(ChatMessage(id: nextID("e"), role: .assistant, content: "Couldn't start a new chat.", isError: true))
                return
            }
            chatID = s.id
            queued.append(t)
            connect(s.id, epoch: epoch)
            return
        }
        if let socket, connected { transmit(t, on: socket) } else { queued.append(t) }
    }

    private func transmit(_ text: String, on socket: ChatSocket) {
        busy = true
        messages.append(ChatMessage(id: nextID("local"), role: .user, content: text, isLocal: true))
        Task {
            do { try await socket.send(text) } catch {
                busy = false
                messages.append(ChatMessage(id: nextID("e"), role: .assistant, content: "Message wasn't delivered. Reconnecting…", isError: true))
            }
        }
    }

    private func connect(_ id: String, epoch e: Int) {
        reader?.cancel()
        socket?.close()
        let s = ChatSocket(client: client, chatID: id)
        socket = s
        reader = Task { [weak self] in
            // The socket has no explicit "open" callback; the first frame or a successful
            // send proves the link. Treat creation as connected and let `done`/close correct it.
            await MainActor.run { guard let self, self.epoch == e else { return }; self.connected = true; self.flushQueue(on: s) }
            for await frame in s.frames {
                guard let self, self.epoch == e else { return }
                self.handle(frame)
            }
            guard let self, self.epoch == e else { return }
            self.connected = false
            self.busy = false
            self.scheduleReconnect(id, epoch: e)
        }
    }

    private func flushQueue(on s: ChatSocket) {
        let q = queued
        queued = []
        for t in q { transmit(t, on: s) }
    }

    private func scheduleReconnect(_ id: String, epoch e: Int) {
        reconnect?.cancel()
        reconnect = Task { [weak self] in
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled, let self, self.epoch == e else { return }
            self.connect(id, epoch: e)
        }
    }

    func handle(_ f: ChatFrame) {
        switch f.kind {
        case .history:
            let past = f.json["messages"].array.enumerated().compactMap { i, m -> ChatMessage? in
                let role: ChatMessage.Role = m["role"].str == "user" ? .user : .assistant
                guard !m["content"].str.isEmpty else { return nil }
                return ChatMessage(id: "history-\(i)", role: role, content: m["content"].str)
            }
            messages = past + messages.filter { $0.isLocal }
        case .message:
            guard !f.content.isEmpty else { return }
            messages.append(ChatMessage(id: nextID("a"), role: .assistant, content: f.content))
        case .error:
            busy = false
            messages.append(ChatMessage(id: nextID("e"), role: .assistant, content: f.content.nonEmpty ?? "Something went wrong", isError: true))
        case .tool_call:
            messages.append(ChatMessage(id: nextID("t"), role: .assistant, content: "",
                                        tool: ToolActivity(name: f.toolName.nonEmpty ?? "tool", input: f.json["input"], status: .running)))
        case .tool_result:
            // Calls finish in order, so resolve the oldest one still running for this tool.
            if let i = messages.firstIndex(where: { $0.tool?.status == .running && $0.tool?.name == f.toolName }) {
                let err = f.json["error"].str
                messages[i].tool?.status = err.isEmpty ? .done : .error
                messages[i].tool?.data = f.json["data"]
                messages[i].tool?.error = err
            }
        case .done:
            busy = false
            for i in messages.indices where messages[i].tool?.status == .running { messages[i].tool?.status = .done }
            onTurnDone?()
        case .action:
            guard let ev = ActionEvent(json: f.json) else { return }
            if let i = actions.firstIndex(where: { $0.id == ev.id }) { actions[i] = ev } else { actions.insert(ev, at: 0) }
        }
    }

    func decide(_ action: ActionEvent, approve: Bool) async {
        do {
            try await client.decide(approval: action.id, approve: approve)
            if let i = actions.firstIndex(where: { $0.id == action.id }) { actions[i].status = approve ? .approved : .failed }
        } catch {
            messages.append(ChatMessage(id: nextID("e"), role: .assistant, content: "Couldn't \(approve ? "approve" : "reject") that action: \(error.localizedDescription)", isError: true))
        }
    }
}
