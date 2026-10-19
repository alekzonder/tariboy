/// <reference types="node" />

import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'

const repoRoot = resolve(process.cwd(), '..')
const source = resolve(repoRoot, 'desktop/branding/tariboy-logo-source.svg')
const iconDir = resolve(repoRoot, 'desktop/src-tauri/icons')

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
})
