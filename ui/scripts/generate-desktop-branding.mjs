import assert from "node:assert/strict";
import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import sharp from "sharp";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "../..");
const brandingDir = path.join(repoRoot, "desktop/branding");
const iconSource = path.join(brandingDir, "white-pixel-t-source.svg");

const outputs = {
  icon: path.join(repoRoot, "desktop/src-tauri/icons/icon.png"),
  icon32: path.join(repoRoot, "desktop/src-tauri/icons/32x32.png"),
  icon64: path.join(repoRoot, "desktop/src-tauri/icons/64x64.png"),
  icon128: path.join(repoRoot, "desktop/src-tauri/icons/128x128.png"),
  icon256: path.join(repoRoot, "desktop/src-tauri/icons/128x128@2x.png"),
  icns: path.join(repoRoot, "desktop/src-tauri/icons/icon.icns"),
};

const pngOptions = {
  compressionLevel: 9,
  adaptiveFiltering: false,
  palette: false,
};

export function encodeIcns(representations) {
  const entries = [];
  for (const [type, png] of representations) {
    assert.match(type, /^[a-zA-Z0-9]{4}$/, "four-byte ICNS type");
    const header = Buffer.alloc(8);
    header.write(type, 0, 4, "ascii");
    header.writeUInt32BE(png.length + 8, 4);
    entries.push(header, png);
  }
  const size = 8 + entries.reduce((sum, entry) => sum + entry.length, 0);
  const header = Buffer.alloc(8);
  header.write("icns", 0, 4, "ascii");
  header.writeUInt32BE(size, 4);
  return Buffer.concat([header, ...entries], size);
}

function runSelfTest() {
  const icns = encodeIcns([["icp4", Buffer.from("png")]]);
  assert.equal(icns.subarray(0, 4).toString("ascii"), "icns");
  assert.equal(icns.readUInt32BE(4), icns.length);
  process.stdout.write("branding self-test ok\n");
}

async function loadSource(source) {
  const decoded = await sharp(source)
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });
  const { width, height, channels } = decoded.info;
  assert.equal(width, 1024, `${source}: width`);
  assert.equal(height, 1024, `${source}: height`);
  assert.equal(channels, 4, `${source}: RGBA channels`);

  let transparent = 0;
  let opaque = 0;
  for (let offset = 3; offset < decoded.data.length; offset += 4) {
    if (decoded.data[offset] === 0) transparent += 1;
    if (decoded.data[offset] === 255) opaque += 1;
  }
  assert(transparent > 0, `${source}: transparent background`);
  assert(opaque > 0, `${source}: opaque foreground`);
  assert.equal(decoded.data[3], 0, `${source}: top-left is transparent`);
  assert.equal(
    decoded.data[((height >> 1) * width + (width >> 1)) * 4 + 3],
    255,
    `${source}: center foreground is opaque`,
  );
  return { rgba: decoded.data, width, height };
}

function pngFromRgba(image, size = image.width) {
  return sharp(image.rgba, {
    raw: { width: image.width, height: image.height, channels: 4 },
  })
    .resize(size, size, {
      fit: "fill",
      kernel: sharp.kernel.lanczos3,
    })
    .png(pngOptions)
    .toBuffer();
}

async function expectedOutputs() {
  const icon = await loadSource(iconSource);
  const sizes = new Map();
  for (const size of [16, 32, 64, 128, 256, 512, 1024]) {
    sizes.set(size, await pngFromRgba(icon, size));
  }
  const icns = encodeIcns([
    ["icp4", sizes.get(16)],
    ["icp5", sizes.get(32)],
    ["icp6", sizes.get(64)],
    ["ic07", sizes.get(128)],
    ["ic08", sizes.get(256)],
    ["ic09", sizes.get(512)],
    ["ic10", sizes.get(1024)],
    ["ic11", sizes.get(32)],
    ["ic12", sizes.get(64)],
    ["ic13", sizes.get(256)],
    ["ic14", sizes.get(512)],
  ]);
  return new Map([
    [outputs.icon, sizes.get(512)],
    [outputs.icon32, sizes.get(32)],
    [outputs.icon64, sizes.get(64)],
    [outputs.icon128, sizes.get(128)],
    [outputs.icon256, sizes.get(256)],
    [outputs.icns, icns],
  ]);
}

async function atomicWrite(destination, data) {
  await mkdir(path.dirname(destination), { recursive: true });
  const temporary = `${destination}.tmp-${process.pid}`;
  await writeFile(temporary, data);
  await rename(temporary, destination);
}

async function generate(checkOnly) {
  const expected = await expectedOutputs();
  for (const [destination, data] of expected) {
    if (checkOnly) {
      let actual;
      try {
        actual = await readFile(destination);
      } catch {
        throw new Error(`branding asset missing: ${path.relative(repoRoot, destination)}`);
      }
      if (!actual.equals(data)) {
        throw new Error(`branding asset drift: ${path.relative(repoRoot, destination)}`);
      }
    } else {
      await atomicWrite(destination, data);
    }
  }
  process.stdout.write(checkOnly ? "branding assets ok\n" : "branding assets generated\n");
}

async function main() {
  const option = process.argv[2] ?? "";
  if (option === "--self-test") {
    runSelfTest();
    return;
  }
  if (option && option !== "--check") {
    throw new Error(`unknown option: ${option}`);
  }
  runSelfTest();
  await generate(option === "--check");
}

if (
  process.argv[1]
  && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href
) {
  main().catch((error) => {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  });
}
