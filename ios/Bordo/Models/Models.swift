import Foundation

struct ChatSession: Identifiable, Hashable {
    let id: String
    var title: String
    var updatedAt: Date?

    init(json: JSON) {
        id = json.first("id", "ID").str
        title = json.first("title", "Title").str
        updatedAt = Dates.parse(json.first("updated_at", "UpdatedAt").str)
    }
    var displayTitle: String { title.isEmpty ? "New chat" : title }
}

struct Project: Identifiable, Hashable {
    let id: String
    let name: String
    let template: String
    let gitRepoURL: String
    let releaseStatus: String
    let releaseRegion: String
    let updatedAt: Date?

    init(json: JSON) {
        id = json.first("id", "ID").str
        name = json.first("name", "Name").str
        template = json.first("template", "Template").str
        gitRepoURL = json.first("git_repo_url", "GitRepoURL").str
        releaseStatus = json.first("release_status", "ReleaseStatus").str
        releaseRegion = json.first("release_region", "ReleaseRegion").str
        updatedAt = Dates.parse(json.first("updated_at", "UpdatedAt").str)
    }
}

struct Release: Identifiable, Hashable {
    let id: String
    let imageTag: String
    let region: String
    let strategy: String
    let status: String
    let createdAt: Date?

    init(json: JSON) {
        id = json.first("id", "ID").str
        imageTag = json.first("image_tag", "ImageTag").str
        region = json.first("region", "Region").str
        strategy = json.first("strategy", "Strategy").str
        status = json.first("status", "Status").str
        createdAt = Dates.parse(json.first("created_at", "CreatedAt").str)
    }
}

struct Region: Identifiable, Hashable {
    var id: String { name }
    let name: String
    let status: String
    let createdAt: Date?

    init(json: JSON) {
        name = json.first("name", "Name").str
        status = json.first("status", "Status").str.nonEmpty ?? "unknown"
        createdAt = Dates.parse(json.first("created_at", "CreatedAt").str)
    }
}

struct NodeHealth: Identifiable, Hashable {
    var id: String { name }
    let name: String
    let ready: Bool
    let lastHeartbeat: Date?

    init(json: JSON) {
        name = json.first("name", "Name").str
        ready = json.first("ready", "Ready").bool ?? false
        lastHeartbeat = Dates.parse(json.first("last_heartbeat", "LastHeartbeat", "lastHeartbeat").str)
    }
}

struct Build: Identifiable, Hashable {
    let id: String
    let status: String
    let imageTag: String
    let error: String
    let createdAt: Date?

    init(json: JSON) {
        id = json.first("id", "ID").str
        status = json.first("status", "Status").str.nonEmpty ?? "unknown"
        imageTag = json.first("image_tag", "ImageTag").str
        error = json.first("error_message", "ErrorMessage").str
        createdAt = Dates.parse(json.first("created_at", "CreatedAt").str)
    }
}

struct Template: Identifiable, Hashable {
    var id: String { name }
    let name: String
    let description: String
    let category: String

    init(json: JSON) {
        name = json["name"].str
        description = json["description"].str
        category = json["category"].str
    }
}

extension String {
    var nonEmpty: String? { isEmpty ? nil : self }
}

enum Dates {
    private static let frac: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter(); f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]; return f
    }()
    private static let plain = ISO8601DateFormatter()

    static func parse(_ s: String) -> Date? {
        guard !s.isEmpty, let d = frac.date(from: s) ?? plain.date(from: s) else { return nil }
        // Go's zero time (0001-01-01) means "unset".
        return d.timeIntervalSince1970 < 1e9 ? nil : d
    }

    static func relative(_ d: Date?) -> String {
        guard let d else { return "—" }
        let s = max(0, Int(Date().timeIntervalSince(d)))
        if s < 60 { return "\(s)s ago" }
        if s < 3600 { return "\(s / 60)m ago" }
        if s < 86400 { return "\(s / 3600)h ago" }
        return "\(s / 86400)d ago"
    }
}
