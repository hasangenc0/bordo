import XCTest
@testable import Bordo

final class DiskUsageTests: XCTestCase {
    func testParsesControlPlaneReading() {
        let d = DiskUsage(json: JSON.parse(#"{"total_bytes":40800000000,"free_bytes":1500000000,"used_percent":96.3,"level":"critical","reported_at":"2026-11-05T10:00:00Z"}"#)!)
        XCTAssertEqual(d?.level, "critical")
        XCTAssertEqual(d?.usedPercent, 96.3)
        XCTAssertEqual(d?.filling, true)
        XCTAssertNotNil(d?.reportedAt)
    }

    func testNoReadingIsNil() {
        // A reconciler that predates disk reporting sends nothing: the UI says so instead of showing 0%.
        XCTAssertNil(DiskUsage(json: .null))
        XCTAssertNil(DiskUsage(json: JSON.parse("{}")!))
        XCTAssertNil(DiskUsage(json: JSON.parse(#"{"total_bytes":0,"free_bytes":0}"#)!))
    }

    func testDerivesPercentWhenMissing() {
        let d = DiskUsage(json: JSON.parse(#"{"total_bytes":100,"free_bytes":25}"#)!)
        XCTAssertEqual(d?.usedPercent, 75)
        XCTAssertEqual(d?.level, "ok")
        XCTAssertEqual(d?.filling, false)
    }

    func testUnreachableIsRed() {
        XCTAssertEqual(StatusBadge.color("unreachable"), .red)
    }
}
