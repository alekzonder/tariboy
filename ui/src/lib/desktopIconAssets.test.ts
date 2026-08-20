/// <reference types="node" />

import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'

const repoRoot = resolve(process.cwd(), '..')
const source = resolve(repoRoot, 'desktop/branding/white-pixel-t-source.svg')
const iconDir = resolve(repoRoot, 'desktop/src-tauri/icons')

describe('desktop icon assets', () => {
  it('ships the editable white pixel T icon source', () => {
    expect(existsSync(source)).toBe(true)
  })

  it('ships transparent white pixel T icons at every generated size', async () => {
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

      expect(pixel(0, 0)[3]).toBe(0)
      expect(pixel(at(0.5), at(0.2))[3]).toBeGreaterThanOrEqual(254)
      expect(pixel(at(0.5), at(0.5))[3]).toBeGreaterThanOrEqual(254)
      expect(pixel(at(0.2), at(0.5))[3]).toBe(0)
      expect(pixel(at(0.5), at(0.9))[3]).toBe(0)

      let neutral = true
      let whiteEnough = true
      let opaqueWhite = true
      for (let offset = 0; offset < data.length; offset += info.channels) {
        const alpha = data[offset + 3]
        if (alpha === 0) continue
        neutral &&= data[offset] === data[offset + 1]
          && data[offset + 1] === data[offset + 2]
        whiteEnough &&= data[offset] >= 254
        opaqueWhite &&= alpha !== 255 || data[offset] === 255
      }
      expect(neutral).toBe(true)
      expect(whiteEnough).toBe(true)
      expect(opaqueWhite).toBe(true)
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
