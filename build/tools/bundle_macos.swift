import AppKit
import CoreGraphics
import Foundation

func renderIcon(size: Int) -> NSImage {
    let s = CGFloat(size)
    let img = NSImage(size: NSSize(width: s, height: s))
    img.lockFocus()
    
    // Transparent canvas
    NSColor.clear.setFill()
    NSRect(x: 0, y: 0, width: s, height: s).fill()
    
    // macOS Big Sur / Sonoma standard squircle proportions:
    // Canvas: s x s
    // Body: 82.4% of canvas (inset by 8.8% on each side)
    // Corner radius: 22.4% of body
    let margin = s * 0.088
    let bodySize = s * 0.824
    let radius = bodySize * 0.224
    let bodyRect = NSRect(x: margin, y: margin, width: bodySize, height: bodySize)
    let path = NSBezierPath(roundedRect: bodyRect, xRadius: radius, yRadius: radius)
    
    // 1. Drop shadow (for size >= 64)
    NSGraphicsContext.saveGraphicsState()
    if s >= 64 {
        let shadow = NSShadow()
        shadow.shadowColor = NSColor(calibratedWhite: 0, alpha: 0.28)
        shadow.shadowOffset = NSSize(width: 0, height: -s * 0.015)
        shadow.shadowBlurRadius = s * 0.035
        shadow.set()
    }
    
    // Background #1C1917
    let bg = NSColor(red: 28/255.0, green: 25/255.0, blue: 23/255.0, alpha: 1.0)
    bg.setFill()
    path.fill()
    NSGraphicsContext.restoreGraphicsState()
    
    // 2. Subtle specular hairline border
    NSGraphicsContext.saveGraphicsState()
    NSColor(calibratedWhite: 1.0, alpha: 0.12).setStroke()
    path.lineWidth = max(1.0, s * 0.002)
    path.stroke()
    NSGraphicsContext.restoreGraphicsState()
    
    // 3. Text "152"
    let text = "152"
    let fontSize = bodySize * 0.44
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
    
    let x = margin + (bodySize - strSize.width) / 2.0
    let y = margin + (bodySize - strSize.height) / 2.0 - (bodySize * 0.02)
    
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

// Always regenerate macOS ICNS and PNG assets
let iconsetPath = "build/icon.iconset"
try? fm.removeItem(atPath: iconsetPath)
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

// Also save standalone PNG icons with squircle
let icon1024 = renderIcon(size: 1024)
savePNG(image: icon1024, to: "build/appicon.png")

let icon512 = renderIcon(size: 512)
savePNG(image: icon512, to: "cmd/app/icon.png")
savePNG(image: icon512, to: "frontend/public/appicon.png")

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

var appVersion = "1.10.3"
if let verContent = try? String(contentsOfFile: "internal/version/version.go", encoding: .utf8) {
    let pattern = "Version = \"([^\"]+)\""
    if let regex = try? NSRegularExpression(pattern: pattern),
       let match = regex.firstMatch(in: verContent, range: NSRange(verContent.startIndex..., in: verContent)),
       let range = Range(match.range(at: 1), in: verContent) {
        appVersion = String(verContent[range])
    }
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
    <string>Деперсонализация ПД</string>
    <key>CFBundleDisplayName</key>
    <string>Деперсонализация ПД</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>\(appVersion)</string>
    <key>CFBundleVersion</key>
    <string>\(appVersion)</string>
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
try? fm.setAttributes([.modificationDate: Date()], ofItemAtPath: appBundlePath)
print("Successfully bundled macOS app and applied squircle icons to bin artifacts")
