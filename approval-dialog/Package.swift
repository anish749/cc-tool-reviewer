// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "approval-dialog",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "approval-dialog",
            path: "Sources"
        )
    ]
)
