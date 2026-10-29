import Foundation

/// Loosely-typed JSON. The control plane serializes some structs with Go field names
/// (PascalCase) and others with snake_case tags, and tool results are free-form,
/// so models are built from this instead of strict Codable structs.
enum JSON: Codable, Equatable, Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSON])
    case object([String: JSON])

    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let v = try? c.decode(Bool.self) { self = .bool(v) }
        else if let v = try? c.decode(Double.self) { self = .number(v) }
        else if let v = try? c.decode(String.self) { self = .string(v) }
        else if let v = try? c.decode([JSON].self) { self = .array(v) }
        else if let v = try? c.decode([String: JSON].self) { self = .object(v) }
        else { throw DecodingError.dataCorruptedError(in: c, debugDescription: "Unsupported JSON value") }
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case .bool(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .object(let v): try c.encode(v)
        }
    }

    subscript(key: String) -> JSON {
        if case .object(let o) = self { return o[key] ?? .null }
        return .null
    }

    subscript(index: Int) -> JSON {
        if case .array(let a) = self, a.indices.contains(index) { return a[index] }
        return .null
    }

    /// First non-null value among the given keys (e.g. "id", "ID").
    func first(_ keys: String...) -> JSON {
        for k in keys { let v = self[k]; if v != .null { return v } }
        return .null
    }

    var string: String? {
        switch self {
        case .string(let s): return s
        case .number(let n): return n == n.rounded() ? String(Int(n)) : String(n)
        case .bool(let b): return String(b)
        default: return nil
        }
    }
    var str: String { string ?? "" }
    var double: Double? { if case .number(let n) = self { return n }; if case .string(let s) = self { return Double(s) }; return nil }
    var bool: Bool? { if case .bool(let b) = self { return b }; return nil }
    var array: [JSON] { if case .array(let a) = self { return a }; return [] }
    var object: [String: JSON]? { if case .object(let o) = self { return o }; return nil }
    var isNull: Bool { self == .null }

    var pretty: String {
        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        guard let d = try? enc.encode(self), let s = String(data: d, encoding: .utf8) else { return "" }
        return s
    }

    static func parse(_ data: Data) -> JSON? { try? JSONDecoder().decode(JSON.self, from: data) }
    static func parse(_ text: String) -> JSON? { parse(Data(text.utf8)) }
}

extension JSON: ExpressibleByDictionaryLiteral, ExpressibleByStringLiteral, ExpressibleByArrayLiteral {
    init(dictionaryLiteral elements: (String, JSON)...) { self = .object(Dictionary(uniqueKeysWithValues: elements)) }
    init(stringLiteral value: String) { self = .string(value) }
    init(arrayLiteral elements: JSON...) { self = .array(elements) }
}
