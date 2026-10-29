import XCTest
import SwiftUI
@testable import Bordo

final class MarkdownSnapshotTests: XCTestCase {
    /// Writes a rendering of a sample reply to $BORDO_SNAPSHOT_PATH for eyeballing; no-op otherwise.
    @MainActor func testRenderSample() throws {
        guard let path = ProcessInfo.processInfo.environment["BORDO_SNAPSHOT_PATH"] else { throw XCTSkip("no BORDO_SNAPSHOT_PATH") }
        let md = """
        ## Your apps
        You have **2 apps** running. See the [docs](https://example.com) or run `bordo status`.

        - hello-api — *healthy* in `eu`
          - last deploy 2h ago
        - [x] worker deployed
        - [ ] add ingress

        1. Build the image
        2. Deploy to a region

        > Heads up: rolling deploys take about a minute.

        | App | Region | Status |
        |---|---|--:|
        | hello-api | eu | running |
        | worker | us | deploying |

        ```bash
        bordo deploy hello-api --region eu
        ```
        ---
        ~~old~~ done.
        """
        let view = MarkdownText(text: md).padding(16).frame(width: 390).background(Color(.systemBackground))
        let r = ImageRenderer(content: view)
        r.scale = 2
        try XCTUnwrap(r.uiImage?.pngData()).write(to: URL(fileURLWithPath: path))
    }
}

extension MarkdownSnapshotTests {
    @MainActor func testRenderComponents() throws {
        guard let path = ProcessInfo.processInfo.environment["BORDO_SNAPSHOT_PATH2"] else { throw XCTSkip("no BORDO_SNAPSHOT_PATH2") }
        let row = HStack(spacing: 12) {
            Avatar(name: "hello-api")
            VStack(alignment: .leading, spacing: 3) {
                Text("hello-api").font(.headline)
                Text("java-web-service · eu").font(.caption).foregroundStyle(.secondary)
            }
            Spacer(minLength: 8)
            StatusBadge(status: "running")
        }.card()
        let tool = ToolCard(tool: ToolActivity(name: "deploy", input: .null, status: .done, data: ["releases": [["region": "eu", "status": "deploying"]]]))
        let running = ToolCard(tool: ToolActivity(name: "code_edit_file", input: ["path": "src/App.java"], status: .running))
        let composer = HStack {
            Text("Ask Bordo…").foregroundStyle(.secondary).padding(.leading, 16)
            Spacer()
            Image(systemName: "arrow.up").font(.system(size: 15, weight: .bold)).foregroundStyle(.white)
                .frame(width: 32, height: 32).background(Theme.gradient, in: Circle()).padding(6)
        }.padding(.vertical, 5)
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
        let content = VStack(alignment: .leading, spacing: 12) {
            HStack { Spacer(); Text("Deploy hello-api to eu").padding(.horizontal, 14).padding(.vertical, 9).background(Theme.gradient, in: RoundedRectangle(cornerRadius: 20, style: .continuous)).foregroundStyle(.white) }
            running; tool
            MarkdownText(text: "Deploying now. I'll check **status** in a minute.")
            row
            row
            composer
        }.padding(16).frame(width: 390)
        for (name, scheme) in [("light", ColorScheme.light), ("dark", .dark)] {
            let v = content.background(Color(scheme == .dark ? .black : .systemGroupedBackground)).environment(\.colorScheme, scheme)
            let r = ImageRenderer(content: v); r.scale = 2
            try XCTUnwrap(r.uiImage?.pngData()).write(to: URL(fileURLWithPath: path.replacingOccurrences(of: ".png", with: "-\(name).png")))
        }
    }
}
