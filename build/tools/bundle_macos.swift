import AppKit
import CoreGraphics
import Foundation

func renderIcon(size: Int) -> NSImage {
    let s = CGFloat(size)
    let img = NSImage(size: NSSize(width: s, height: s))
    img.lockFocus()
    
    // Background #1C1917
    let bg = NSColor(red: 28/255.0, green: 25/255.0, blue: 23/255.0, alpha: 1.0)
    bg.setFill()
    NSRect(x: 0, y: 0, width: s, height: s).fill()
    
    // Text "152"
    let text = "152"
    let fontSize = s * 0.47
    let font = NSFont.monospacedSystemFont(ofSize: fontSize, weight: .heavy)
    
    let paragraphStyle = NSMutableParagraphStyle()
    paragraphStyle.alignment = .center
    
    let attrs: [NSAttributedString.Key: Any] = [
        .font: font,
        .foregroundColor: NSColor.white,
        .paragraphStyle: paragraphStyle
    ]
    
    let attrStr = NSAttributedString(string: text, attributes: attrs)
    let strSize = attrStr.size()
    
    let x = (s - strSize.width) / 2.0
    let y = (s - strSize.height) / 2.0 - (s * 0.02)
    
    attrStr.draw(in: NSRect(x: x, y: y, width: strSize.width, height: strSize.height))
    
    img.unlockFocus()
    return img
}

func savePNG(image: NSImage, to path: String) {
    guard let tiff = image.tiffRepresentation,
          let rep = NSBitmapImageRep(data: tiff),
          let png = rep.representation(using: .png, properties: [:]) else {
        return
    }
    try? png.write(to: URL(fileURLWithPath: path))
}

let fm = FileManager.default

// Create ICNS if not exists
if !fm.fileExists(atPath: "build/appicon.icns") {
    let iconsetPath = "build/icon.iconset"
    try? fm.createDirectory(atPath: iconsetPath, withIntermediateDirectories: true)

    let iconSizes: [(name: String, size: Int)] = [
        ("icon_16x16.png", 16),
        ("icon_16x16@2x.png", 32),
        ("icon_32x32.png", 32),
        ("icon_32x32@2x.png", 64),
        ("icon_128x128.png", 128),
        ("icon_128x128@2x.png", 256),
        ("icon_256x256.png", 256),
        ("icon_256x256@2x.png", 512),
        ("icon_512x512.png", 512),
        ("icon_512x512@2x.png", 1024)
    ]

    for item in iconSizes {
        let img = renderIcon(size: item.size)
        savePNG(image: img, to: "\(iconsetPath)/\(item.name)")
    }

    let process = Process()
    process.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
    process.arguments = ["-c", "icns", iconsetPath, "-o", "build/appicon.icns"]
    try? process.run()
    process.waitUntilExit()

    try? fm.removeItem(atPath: iconsetPath)
}

// Create macOS .app bundle in bin/Depersonalizer.app
let appBundlePath = "bin/Depersonalizer.app"
let macosDir = "\(appBundlePath)/Contents/MacOS"
let resourcesDir = "\(appBundlePath)/Contents/Resources"

try? fm.createDirectory(atPath: macosDir, withIntermediateDirectories: true)
try? fm.createDirectory(atPath: resourcesDir, withIntermediateDirectories: true)

try? fm.removeItem(atPath: "\(resourcesDir)/icon.icns")
try? fm.copyItem(atPath: "build/appicon.icns", toPath: "\(resourcesDir)/icon.icns")

if fm.fileExists(atPath: "bin/depersonalizer-darwin-universal") {
    let destBinary = "\(macosDir)/depersonalizer"
    try? fm.removeItem(atPath: destBinary)
    try? fm.copyItem(atPath: "bin/depersonalizer-darwin-universal", toPath: destBinary)
    try? fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: destBinary)
}

let infoPlist = """
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>depersonalizer</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>ru.depersonalizer.app</string>
    <key>CFBundleName</key>
    <string>Деперсонализация ПД (152-ФЗ)</string>
    <key>CFBundleDisplayName</key>
    <string>Деперсонализация ПД (152-ФЗ)</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.6.0</string>
    <key>CFBundleVersion</key>
    <string>1.6.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
"""
try? infoPlist.write(to: URL(fileURLWithPath: "\(appBundlePath)/Contents/Info.plist"), atomically: true, encoding: .utf8)
try? "APPL????".write(to: URL(fileURLWithPath: "\(appBundlePath)/Contents/PkgInfo"), atomically: true, encoding: .utf8)

// Set custom Finder icons
let icon512 = renderIcon(size: 512)
let ws = NSWorkspace.shared

let filesToIcon = [
    "bin/depersonalizer",
    "bin/depersonalizer-darwin-universal",
    "bin/depersonalizer-altlinux-amd64",
    "bin/depersonalizer-windows-amd64.exe",
    appBundlePath
]

for f in filesToIcon {
    if fm.fileExists(atPath: f) {
        _ = ws.setIcon(icon512, forFile: f, options: [])
    }
}
print("Successfully bundled macOS app and applied icons to bin artifacts")
