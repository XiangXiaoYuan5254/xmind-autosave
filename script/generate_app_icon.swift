#!/usr/bin/env swift

import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

guard CommandLine.arguments.count == 2 else {
    FileHandle.standardError.write(Data("usage: generate_app_icon.swift <output.iconset>\n".utf8))
    exit(2)
}

let fileManager = FileManager.default
let outputURL = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try? fileManager.removeItem(at: outputURL)
try fileManager.createDirectory(at: outputURL, withIntermediateDirectories: true)

let variants: [(pixels: Int, name: String)] = [
    (16, "icon_16x16.png"),
    (32, "icon_16x16@2x.png"),
    (32, "icon_32x32.png"),
    (64, "icon_32x32@2x.png"),
    (128, "icon_128x128.png"),
    (256, "icon_128x128@2x.png"),
    (256, "icon_256x256.png"),
    (512, "icon_256x256@2x.png"),
    (512, "icon_512x512.png"),
    (1_024, "icon_512x512@2x.png")
]

func roundedRect(_ rect: CGRect, radius: CGFloat) -> CGPath {
    CGPath(roundedRect: rect, cornerWidth: radius, cornerHeight: radius, transform: nil)
}

func makeIcon(pixels: Int) throws -> CGImage {
    let size = CGFloat(pixels)
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    guard let context = CGContext(
        data: nil,
        width: pixels,
        height: pixels,
        bitsPerComponent: 8,
        bytesPerRow: 0,
        space: colorSpace,
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    ) else {
        throw CocoaError(.fileWriteUnknown)
    }

    context.interpolationQuality = .high
    context.setAllowsAntialiasing(true)

    let tileRect = CGRect(x: size * 0.055, y: size * 0.055, width: size * 0.89, height: size * 0.89)
    let tilePath = roundedRect(tileRect, radius: size * 0.215)
    context.saveGState()
    context.setShadow(
        offset: CGSize(width: 0, height: -size * 0.022),
        blur: size * 0.045,
        color: CGColor(gray: 0.12, alpha: 0.22)
    )
    context.addPath(tilePath)
    context.setFillColor(CGColor(red: 0.93, green: 0.965, blue: 0.95, alpha: 1))
    context.fillPath()
    context.restoreGState()

    context.saveGState()
    context.addPath(tilePath)
    context.clip()
    let gradient = CGGradient(
        colorsSpace: colorSpace,
        colors: [
            CGColor(red: 0.965, green: 0.98, blue: 0.972, alpha: 1),
            CGColor(red: 0.83, green: 0.905, blue: 0.87, alpha: 1)
        ] as CFArray,
        locations: [0, 1]
    )!
    context.drawLinearGradient(
        gradient,
        start: CGPoint(x: size * 0.5, y: tileRect.maxY),
        end: CGPoint(x: size * 0.5, y: tileRect.minY),
        options: []
    )
    context.restoreGState()

    context.addPath(tilePath)
    context.setStrokeColor(CGColor(red: 0.25, green: 0.39, blue: 0.33, alpha: 0.23))
    context.setLineWidth(size * 0.012)
    context.strokePath()

    let trackRect = CGRect(x: size * 0.18, y: size * 0.35, width: size * 0.64, height: size * 0.30)
    let trackPath = roundedRect(trackRect, radius: trackRect.height / 2)
    context.saveGState()
    context.setShadow(
        offset: CGSize(width: 0, height: -size * 0.008),
        blur: size * 0.018,
        color: CGColor(gray: 0.1, alpha: 0.18)
    )
    context.addPath(trackPath)
    context.setFillColor(CGColor(red: 0.38, green: 0.56, blue: 0.48, alpha: 1))
    context.fillPath()
    context.restoreGState()

    let knobInset = size * 0.028
    let knobDiameter = trackRect.height - knobInset * 2
    let knobRect = CGRect(
        x: trackRect.maxX - knobDiameter - knobInset,
        y: trackRect.minY + knobInset,
        width: knobDiameter,
        height: knobDiameter
    )
    context.saveGState()
    context.setShadow(
        offset: CGSize(width: 0, height: -size * 0.008),
        blur: size * 0.018,
        color: CGColor(gray: 0.05, alpha: 0.30)
    )
    context.addEllipse(in: knobRect)
    context.setFillColor(CGColor(gray: 0.99, alpha: 1))
    context.fillPath()
    context.restoreGState()

    guard let image = context.makeImage() else {
        throw CocoaError(.fileWriteUnknown)
    }
    return image
}

for variant in variants {
    let image = try makeIcon(pixels: variant.pixels)
    let destinationURL = outputURL.appendingPathComponent(variant.name)
    guard let destination = CGImageDestinationCreateWithURL(
        destinationURL as CFURL,
        UTType.png.identifier as CFString,
        1,
        nil
    ) else {
        throw CocoaError(.fileWriteUnknown)
    }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else {
        throw CocoaError(.fileWriteUnknown)
    }
}
