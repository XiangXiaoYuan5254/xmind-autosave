import Foundation
import Testing
@testable import XMindAutoSaveCore

private let releaseResponse = Data(#"""
{
  "tag_name": "v1.0.4",
  "html_url": "https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/tag/v1.0.4",
  "assets": [
    {"name": "SHA256SUMS.txt"},
    {"name": "XMindAutoSave-1.0.4.dmg"},
    {"name": "XMindAutoSave-Setup-1.0.4.exe"}
  ]
}
"""#.utf8)

@Test("解析版本号，开发版本不算版本")
func parsesVersions() {
    #expect(AppVersion("1.0.3")?.description == "1.0.3")
    #expect(AppVersion("v1.0.4")?.description == "1.0.4")
    #expect(AppVersion("V1.2.3.4")?.description == "1.2.3.4")
    for text in ["", "dev", "ci-abc1234", "1.0.3-beta", "1..3", "1.2.3.4.5", "v", "+1.0"] {
        #expect(AppVersion(text) == nil, "\(text)")
    }
}

@Test("按数字比较版本号")
func comparesVersions() throws {
    #expect(try #require(AppVersion("1.0.3")) < #require(AppVersion("1.0.4")))
    #expect(try #require(AppVersion("1.0.10")) > #require(AppVersion("1.0.9")))
    #expect(try #require(AppVersion("1.1")) == #require(AppVersion("1.1.0")))
    #expect(try #require(AppVersion("2.0.0")) > #require(AppVersion("1.99.99")))
}

@Test("有更新的版本时提示")
func offersNewerRelease() throws {
    let update = try UpdateCheck.availableUpdate(
        inLatestRelease: releaseResponse,
        currentVersion: #require(AppVersion("1.0.3"))
    )
    #expect(update?.version.description == "1.0.4")
    #expect(update?.pageURL.absoluteString == "https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/tag/v1.0.4")
}

@Test("已是最新或更新时不提示")
func ignoresSameOrOlderRelease() throws {
    for current in ["1.0.4", "1.1.0"] {
        let update = try UpdateCheck.availableUpdate(
            inLatestRelease: releaseResponse,
            currentVersion: #require(AppVersion(current))
        )
        #expect(update == nil, "\(current)")
    }
}

@Test("没有 DMG 的版本不提示")
func needsMacInstaller() throws {
    let windowsOnly = Data(#"{"tag_name": "v1.0.4", "assets": [{"name": "XMindAutoSave-Setup-1.0.4.exe"}]}"#.utf8)
    let update = try UpdateCheck.availableUpdate(
        inLatestRelease: windowsOnly,
        currentVersion: #require(AppVersion("1.0.3"))
    )
    #expect(update == nil)
}

@Test("只打开本项目的下载页面")
func opensOnlyThisProjectsPages() throws {
    let elsewhere = Data(#"{"tag_name": "v1.0.4", "html_url": "https://example.com/download", "assets": [{"name": "XMindAutoSave-1.0.4.dmg"}]}"#.utf8)
    let update = try UpdateCheck.availableUpdate(
        inLatestRelease: elsewhere,
        currentVersion: #require(AppVersion("1.0.3"))
    )
    #expect(update?.pageURL == UpdateCheck.latestReleasePage)
}

@Test("无法识别的版本号报错")
func rejectsUnknownTag() throws {
    let current = try #require(AppVersion("1.0.3"))
    #expect(throws: (any Error).self) {
        try UpdateCheck.availableUpdate(inLatestRelease: Data(#"{"tag_name": "nightly"}"#.utf8), currentVersion: current)
    }
}

@Test(
    "向 GitHub 查询最新版本（设置 XMIND_AUTOSAVE_NETWORK_TESTS=1 时运行）",
    .enabled(if: ProcessInfo.processInfo.environment["XMIND_AUTOSAVE_NETWORK_TESTS"] == "1")
)
func fetchesLatestReleaseFromGitHub() async throws {
    let update = try await UpdateCheck.fetchAvailableUpdate(
        currentVersion: #require(AppVersion("0.0.1")),
        session: URLSession(configuration: .ephemeral)
    )
    let found = try #require(update)
    print("最新版本：\(found.version)，\(found.pageURL)")
}
