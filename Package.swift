// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "XMindAutoSave",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(name: "XMindAutoSave", targets: ["XMindAutoSave"]),
        .library(name: "XMindAutoSaveCore", targets: ["XMindAutoSaveCore"])
    ],
    targets: [
        .target(name: "XMindAutoSaveCore"),
        .executableTarget(
            name: "XMindAutoSave",
            dependencies: ["XMindAutoSaveCore"]
        ),
        .testTarget(
            name: "XMindAutoSaveCoreTests",
            dependencies: ["XMindAutoSaveCore"]
        )
    ]
)
