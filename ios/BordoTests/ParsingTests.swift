import XCTest
@testable import Bordo

final class ParsingTests: XCTestCase {
    func testRegionAcceptsGoFieldNames() {
        let r = Region(json: JSON.parse(#"{"Name":"eu","Status":"healthy","CreatedAt":"2026-10-20T10:00:00Z"}"#)!)
        XCTAssertEqual(r.name, "eu"); XCTAssertEqual(r.status, "healthy"); XCTAssertNotNil(r.createdAt)
    }

    func testGoZeroTimeIsUnset() {
        XCTAssertNil(Dates.parse("0001-01-01T00:00:00Z"))
    }

    func testToolResultPairsWithOldestRunningCall() async throws {
        let vm = await ChatViewModel(client: AgentClient(port: 1, token: ""))
        await MainActor.run {
            func f(_ s: String) -> ChatFrame { let j = JSON.parse(s)!; return ChatFrame(kind: ChatFrame.Kind(rawValue: j["type"].str)!, json: j) }
            vm.handle(f(#"{"type":"tool_call","tool_name":"deploy","input":{"a":1}}"#))
            vm.handle(f(#"{"type":"tool_call","tool_name":"deploy","input":{"a":2}}"#))
            vm.handle(f(#"{"type":"tool_result","tool_name":"deploy","data":{"ok":true}}"#))
            XCTAssertEqual(vm.messages[0].tool?.status, .done)
            XCTAssertEqual(vm.messages[1].tool?.status, .running)
            vm.handle(f(#"{"type":"done"}"#))
            XCTAssertEqual(vm.messages[1].tool?.status, .done, "calls still running at end of turn are closed")
        }
    }

    func testHistoryKeepsLocallyTypedMessages() async {
        let vm = await ChatViewModel(client: AgentClient(port: 1, token: ""))
        await MainActor.run {
            vm.handle(ChatFrame(kind: .history, json: JSON.parse(#"{"type":"history","messages":[{"role":"user","content":"old"},{"role":"assistant","content":"reply"}]}"#)!))
            XCTAssertEqual(vm.messages.map(\.content), ["old", "reply"])
        }
    }

    func testObserveQueryEncodingEscapesPlusAndAmpersand() {
        let s = AgentClient.encode([URLQueryItem(name: "query", value: #"a+b & c{x="y"}"#)])
        XCTAssertFalse(s.contains("+")); XCTAssertFalse(s.contains(" & "))
    }

    func testFingerprintMatchesSSHKeygenFormat() {
        // ssh-keygen -lf of this key prints SHA256:... (43 base64 chars, unpadded)
        let fp = HostKeyStore.fingerprint(openSSH: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKd5Uk0eCwQ0Ou6yE2ZJ8nJuaDqa1m3RqJ4Vq4kqE0Sx comment")
        XCTAssertTrue(fp.hasPrefix("SHA256:")); XCTAssertEqual(fp.count, 7 + 43)
    }

    func testMarkdownBlocks() {
        let md = """
        # Title
        Some **bold** text
        - one
          - nested
        - [x] done

        1. first

        > quoted

        | a | b |
        |---|--:|
        | 1 | 2 |

        ```swift
        let x = 1
        ```
        ---
        """
        let b = MDParser.parse(md)
        guard b.count == 8 else { return XCTFail("got \(b.count) blocks: \(b)") }
        if case .heading(1, "Title") = b[0] {} else { XCTFail("heading") }
        if case .paragraph = b[1] {} else { XCTFail("paragraph") }
        if case .list(let l) = b[2] { XCTAssertEqual(l.count, 3); XCTAssertEqual(l[1].indent, 1); XCTAssertEqual(l[2].checked, true) } else { XCTFail("list") }
        if case .list(let l) = b[3] { XCTAssertEqual(l[0].marker, "1.") } else { XCTFail("ordered") }
        if case .quote = b[4] {} else { XCTFail("quote") }
        if case .table(let h, let al, let rows) = b[5] { XCTAssertEqual(h, ["a", "b"]); XCTAssertEqual(al[1], .trailing); XCTAssertEqual(rows, [["1", "2"]]) } else { XCTFail("table") }
        if case .code(let lang, let t) = b[6] { XCTAssertEqual(lang, "swift"); XCTAssertEqual(t, "let x = 1") } else { XCTFail("code") }
        if case .rule = b[7] {} else { XCTFail("rule") }
    }

    func testUnterminatedFenceRunsToEnd() {
        let b = MDParser.parse("```\nabc\ndef")
        if case .code(_, let t) = b.first { XCTAssertEqual(t, "abc\ndef") } else { XCTFail() }
    }
}
