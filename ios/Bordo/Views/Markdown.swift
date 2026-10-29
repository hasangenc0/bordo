import SwiftUI

/// Block-level Markdown model. SwiftUI's `AttributedString(markdown:)` only handles inline
/// syntax well, so blocks (headings, lists, tables, quotes, code) are parsed here and each
/// block's text is rendered with the inline parser.
indirect enum MDBlock: Identifiable {
    case heading(Int, String)
    case paragraph(String)
    case list([MDItem])
    case quote([MDBlock])
    case code(lang: String, text: String)
    case table(header: [String], aligns: [TextAlignment], rows: [[String]])
    case rule

    var id: UUID { UUID() }
}

struct MDItem: Identifiable {
    let id = UUID()
    let indent: Int
    let marker: String      // "•", "1.", or a task box
    let text: String
    let checked: Bool?
}

enum MDParser {
    static func parse(_ text: String) -> [MDBlock] {
        parse(lines: text.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n"))
    }

    private static func parse(lines: [String]) -> [MDBlock] {
        var out: [MDBlock] = []
        var para: [String] = []
        var i = 0

        func flush() {
            if !para.isEmpty { out.append(.paragraph(para.joined(separator: "\n"))); para = [] }
        }

        while i < lines.count {
            let line = lines[i]
            let t = line.trimmingCharacters(in: .whitespaces)

            if t.hasPrefix("```") || t.hasPrefix("~~~") {
                flush()
                let fence = String(t.prefix(3))
                let lang = String(t.dropFirst(3)).trimmed
                var body: [String] = []
                i += 1
                while i < lines.count, !lines[i].trimmingCharacters(in: .whitespaces).hasPrefix(fence) { body.append(lines[i]); i += 1 }
                i += 1 // closing fence (an unterminated block just runs to the end)
                out.append(.code(lang: lang, text: body.joined(separator: "\n")))
                continue
            }

            if t.isEmpty { flush(); i += 1; continue }

            if t.range(of: #"^(-{3,}|\*{3,}|_{3,})$"#, options: .regularExpression) != nil {
                flush(); out.append(.rule); i += 1; continue
            }

            if let m = t.range(of: #"^#{1,6}\s+"#, options: .regularExpression) {
                flush()
                let level = t[m].filter { $0 == "#" }.count
                let body = String(t[m.upperBound...]).replacingOccurrences(of: #"\s+#+\s*$"#, with: "", options: .regularExpression)
                out.append(.heading(level, body))
                i += 1; continue
            }

            if t.hasPrefix(">") {
                flush()
                var q: [String] = []
                while i < lines.count, lines[i].trimmingCharacters(in: .whitespaces).hasPrefix(">") {
                    var l = lines[i].trimmingCharacters(in: .whitespaces).dropFirst()
                    if l.hasPrefix(" ") { l = l.dropFirst() }
                    q.append(String(l)); i += 1
                }
                out.append(.quote(parse(lines: q)))
                continue
            }

            // Table: a header row followed by a |---|---| separator row.
            if t.contains("|"), i + 1 < lines.count, let aligns = tableAligns(lines[i + 1]) {
                flush()
                let header = cells(t)
                var rows: [[String]] = []
                i += 2
                while i < lines.count, lines[i].contains("|"), !lines[i].trimmed.isEmpty { rows.append(cells(lines[i])); i += 1 }
                out.append(.table(header: header, aligns: aligns, rows: rows))
                continue
            }

            if item(line) != nil {
                flush()
                var items: [MDItem] = []
                while i < lines.count {
                    if let it = item(lines[i]) { items.append(it); i += 1 }
                    // A wrapped continuation line belongs to the previous item.
                    else if !lines[i].trimmed.isEmpty, lines[i].hasPrefix("  "), !items.isEmpty, !lines[i].trimmingCharacters(in: .whitespaces).hasPrefix("```") {
                        let last = items.removeLast()
                        items.append(MDItem(indent: last.indent, marker: last.marker, text: last.text + "\n" + lines[i].trimmed, checked: last.checked))
                        i += 1
                    } else { break }
                }
                out.append(.list(items))
                continue
            }

            para.append(line)
            i += 1
        }
        flush()
        return out
    }

    private static func item(_ line: String) -> MDItem? {
        guard let m = line.range(of: #"^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$"#, options: .regularExpression) else { return nil }
        let s = String(line[m])
        let lead = s.prefix { $0 == " " || $0 == "\t" }
        let rest = s.dropFirst(lead.count)
        let marker = String(rest.prefix { !$0.isWhitespace })
        var body = String(rest.dropFirst(marker.count)).trimmingCharacters(in: .whitespaces)
        var checked: Bool?
        if body.hasPrefix("[ ] ") { checked = false; body = String(body.dropFirst(4)) }
        else if body.lowercased().hasPrefix("[x] ") { checked = true; body = String(body.dropFirst(4)) }
        let indent = lead.reduce(0) { $0 + ($1 == "\t" ? 2 : 1) } / 2
        let shown = marker.first?.isNumber == true ? marker : "•"
        return MDItem(indent: min(indent, 4), marker: shown, text: body, checked: checked)
    }

    private static func cells(_ line: String) -> [String] {
        var t = line.trimmed
        if t.hasPrefix("|") { t.removeFirst() }
        if t.hasSuffix("|") { t.removeLast() }
        return t.components(separatedBy: "|").map { $0.trimmed }
    }

    private static func tableAligns(_ line: String) -> [TextAlignment]? {
        let c = cells(line)
        guard !c.isEmpty, c.allSatisfy({ $0.range(of: #"^:?-{1,}:?$"#, options: .regularExpression) != nil }) else { return nil }
        return c.map { $0.hasPrefix(":") && $0.hasSuffix(":") ? .center : $0.hasSuffix(":") ? .trailing : .leading }
    }
}

struct MarkdownText: View {
    let text: String
    private let blocks: [MDBlock]

    init(text: String) {
        self.text = text
        blocks = MDCache.blocks(text)
    }

    var body: some View { MDBlocks(blocks: blocks) }
}

private struct MDBlocks: View {
    let blocks: [MDBlock]

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, b in block(b) }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private func block(_ b: MDBlock) -> some View {
        switch b {
        case .heading(let level, let t):
            Text(MD.inline(t))
                .font(level == 1 ? .title2.bold() : level == 2 ? .title3.bold() : level == 3 ? .headline : .subheadline.bold())
                .padding(.top, level <= 2 ? 6 : 2)
                .textSelection(.enabled)
        case .paragraph(let t):
            Text(MD.inline(t)).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
        case .list(let items):
            VStack(alignment: .leading, spacing: 4) {
                ForEach(items) { it in
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Group {
                            if let c = it.checked { Image(systemName: c ? "checkmark.square.fill" : "square").foregroundStyle(c ? Theme.accent : .secondary) }
                            else { Text(it.marker).foregroundStyle(.secondary).monospacedDigit() }
                        }
                        .frame(minWidth: 16, alignment: .trailing)
                        Text(MD.inline(it.text)).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    }
                    .padding(.leading, CGFloat(it.indent) * 16)
                }
            }
        case .quote(let inner):
            HStack(alignment: .top, spacing: 10) {
                RoundedRectangle(cornerRadius: 2).fill(Theme.accent.opacity(0.5)).frame(width: 3)
                MDBlocks(blocks: inner).foregroundStyle(.secondary)
            }
        case .code(let lang, let t):
            CodeBlock(text: t, title: lang.isEmpty ? nil : lang)
        case .table(let header, let aligns, let rows):
            ScrollView(.horizontal, showsIndicators: false) {
                Grid(alignment: .leading, horizontalSpacing: 0, verticalSpacing: 0) {
                    GridRow { ForEach(Array(header.enumerated()), id: \.offset) { i, h in cell(h, aligns, i, bold: true) } }
                        .background(Color(.tertiarySystemBackground))
                    ForEach(Array(rows.enumerated()), id: \.offset) { _, r in
                        Divider()
                        GridRow { ForEach(0..<header.count, id: \.self) { i in cell(i < r.count ? r[i] : "", aligns, i, bold: false) } }
                    }
                }
                .background(Color(.secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color(.separator), lineWidth: 0.5))
            }
        case .rule:
            Divider()
        }
    }

    private func cell(_ t: String, _ aligns: [TextAlignment], _ i: Int, bold: Bool) -> some View {
        let a = i < aligns.count ? aligns[i] : .leading
        return Text(MD.inline(t))
            .font(.footnote.weight(bold ? .semibold : .regular))
            .multilineTextAlignment(a)
            .frame(minWidth: 70, maxWidth: 260, alignment: a == .trailing ? .trailing : a == .center ? .center : .leading)
            .padding(.horizontal, 10).padding(.vertical, 6)
            .textSelection(.enabled)
    }
}

/// Parsed blocks per message text, so re-rendering a row never re-parses it.
enum MDCache {
    private final class Box { let blocks: [MDBlock]; init(_ b: [MDBlock]) { blocks = b } }
    private static let cache: NSCache<NSString, Box> = { let c = NSCache<NSString, Box>(); c.countLimit = 300; return c }()

    static func blocks(_ text: String) -> [MDBlock] {
        let k = text as NSString
        if let hit = cache.object(forKey: k) { return hit.blocks }
        let parsed = MDParser.parse(text)
        cache.setObject(Box(parsed), forKey: k)
        return parsed
    }
}

enum MD {
    /// Inline Markdown (bold, italic, strike, links, code) with code spans in monospace on a tint.
    static func inline(_ s: String) -> AttributedString {
        var a = (try? AttributedString(markdown: s, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(s)
        for run in a.runs {
            if run.inlinePresentationIntent?.contains(.code) == true {
                a[run.range].font = .system(.footnote, design: .monospaced)
                a[run.range].backgroundColor = Color(.tertiarySystemFill)
            }
            if run.link != nil {
                a[run.range].foregroundColor = Theme.accent
                a[run.range].underlineStyle = .single
            }
        }
        return a
    }
}
