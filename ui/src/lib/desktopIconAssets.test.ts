/// <reference types="node" />

import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'

const repoRoot = resolve(process.cwd(), '..')
const source = resolve(repoRoot, 'desktop/branding/tariboy-logo-source.svg')
const iconDir = resolve(repoRoot, 'desktop/src-tauri/icons')
const androidRes = resolve(repoRoot, 'desktop/src-tauri/gen/android/app/src/main/res')

describe('desktop icon assets', () => {
  it('ships the editable Tariboy dancer logo source', () => {
    expect(existsSync(source)).toBe(true)
  })

  it('ships the white dancer on a dark rounded tile at every generated size', async () => {
    const sizes = new Map([
      ['icon.png', 512],
      ['32x32.png', 32],
      ['64x64.png', 64],
      ['128x128.png', 128],
      ['128x128@2x.png', 256],
    ])

    for (const [name, size] of sizes) {
      const png = readFileSync(resolve(iconDir, name))
      const { data, info } = await sharp(png)
        .ensureAlpha()
        .raw()
        .toBuffer({ resolveWithObject: true })
      expect(info).toMatchObject({ width: size, height: size, channels: 4 })

      const pixel = (x: number, y: number) => {
        const offset = (y * info.width + x) * info.channels
        return data.subarray(offset, offset + info.channels)
      }
      const at = (ratio: number) => Math.floor(size * ratio)

      const isTile = (x: number, y: number) => {
        const [r, g, b, a] = pixel(x, y)
        return a >= 254 && r <= 40 && g <= 40 && b <= 40
      }
      const isMark = (x: number, y: number) => {
        const [r, g, b, a] = pixel(x, y)
        return a >= 254 && r >= 200 && g >= 200 && b >= 200
      }

      // Rounded tile: transparent corner, dark background around the mark.
      expect(pixel(0, 0)[3]).toBe(0)
      expect(isTile(at(0.5), at(0.1))).toBe(true)
      expect(isTile(at(0.2), at(0.85))).toBe(true)
      expect(isTile(at(0.5), at(0.95))).toBe(true)
      // Head dot, arms arc and body of the dancer.
      expect(isMark(at(0.5), at(0.234))).toBe(true)
      expect(isMark(at(0.5), at(0.5))).toBe(true)
      expect(isMark(at(0.5), at(0.7))).toBe(true)
      // Gap between the head dot and the arms arc.
      expect(isTile(at(0.5), at(0.33))).toBe(true)
    }

    const sourceImage = sharp(source)
    const sourceMetadata = await sourceImage.metadata()
    expect(sourceMetadata).toMatchObject(
      { width: 1024, height: 1024, channels: 4 },
    )

    const icns = readFileSync(resolve(iconDir, 'icon.icns'))
    expect(icns.subarray(0, 4).toString('ascii')).toBe('icns')
  })

  it('does not ship splash assets', () => {
    expect(existsSync(resolve(process.cwd(), 'public/splash.html'))).toBe(false)
    expect(existsSync(resolve(process.cwd(), 'public/splash-logo.png'))).toBe(
      false,
    )
  })

  it.each([
    ['mdpi', 48, 108],
    ['hdpi', 72, 162],
    ['xhdpi', 96, 216],
    ['xxhdpi', 144, 324],
    ['xxxhdpi', 192, 432],
  ] as const)('ships the Android dancer icon at %s density', async (density, size, foregroundSize) => {
    for (const name of ['ic_launcher.png', 'ic_launcher_round.png', 'ic_launcher_foreground.png']) {
      const foreground = name === 'ic_launcher_foreground.png'
      const canvas = foreground ? foregroundSize : size
      const markSize = foreground ? canvas * 2 / 3 : canvas
      const inset = (canvas - markSize) / 2
      const { data, info } = await sharp(resolve(androidRes, `mipmap-${density}`, name))
        .ensureAlpha().raw().toBuffer({ resolveWithObject: true })
      expect(info).toMatchObject({ width: canvas, height: canvas, channels: 4 })
      expect(data[3]).toBe(0)
      const pixel = (x: number, y: number) => {
        const offset = (Math.floor(inset + markSize * y) * canvas + Math.floor(inset + markSize * x)) * 4
        return [...data.subarray(offset, offset + 4)]
      }
      for (const [x, y] of [[0.5, 0.1], [0.2, 0.85], [0.5, 0.95], [0.5, 0.33]]) {
        const [r, g, b, a] = pixel(x, y)
        // Downsampling softens the tile edge at mdpi.
        expect(a).toBeGreaterThanOrEqual(250)
        expect(Math.max(r, g, b)).toBeLessThanOrEqual(40)
      }
      for (const y of [0.234, 0.5, 0.7]) {
        const [r, g, b, a] = pixel(0.5, y)
        expect(a).toBeGreaterThanOrEqual(254)
        expect(Math.min(r, g, b)).toBeGreaterThanOrEqual(200)
      }
      if (foreground) {
        // The adaptive foreground must leave room for launcher masks and motion.
        expect(data[(Math.floor(canvas / 2) * canvas + Math.floor(inset / 2)) * 4 + 3]).toBe(0)
      }
    }
  })

  it('matches the adaptive background to the dark logo tile', () => {
    expect(readFileSync(resolve(androidRes, 'values/ic_launcher_background.xml'), 'utf8'))
      .toMatch(/<color name="ic_launcher_background">#111214<\/color>/)
  })
})
