import Foundation

struct ToolActivity: Equatable {
    enum Status { case running, done, error }
    var name: String
    var input: JSON
    var status: Status
    var data: JSON = .null
    var error: String = ""
}

struct ChatMessage: Identifiable, Equatable {
    enum Role { case user, assistant }
    let id: String
    let role: Role
    var content: String
    var isError = false
    var tool: ToolActivity?
    /// True for messages the user typed in this session (kept when history arrives late).
    var isLocal = false
}

struct ActionEvent: Identifiable, Equatable {
    enum Status: String { case auto_approved, approved, pending, failed }
    let id: String
    var kind: String
    var target: String
    var status: Status
    var ts: Date

    init?(json: JSON) {
        let id = json["action_id"].str
        guard !id.isEmpty else { return nil }
        self.id = id
        kind = json["action_kind"].string ?? "action"
        target = json["action_target"].str
        status = Status(rawValue: json["action_status"].str) ?? .auto_approved
        ts = json["action_ts"].double.map { Date(timeIntervalSince1970: $0 > 1e12 ? $0 / 1000 : $0) } ?? Date()
    }
}
