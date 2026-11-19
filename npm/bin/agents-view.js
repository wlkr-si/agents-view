#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');
const { createHash } = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const path = require('node:path');
const zlib = require('node:zlib');
const { version } = require('../package.json');

const mirror = process.env.AGENTS_VIEW_DOWNLOAD_BASE_URL;

function download(url, limit, redirects = 5) {
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
  const client = url.protocol === 'https:' ? https : mirror && loopback && url.protocol === 'http:' ? http : null;
  if (!client) throw new Error('Release downloads require HTTPS (HTTP is available for loopback mirrors)');

  return new Promise((resolve, reject) => {
    const request = client.get(url, (response) => {
      response.on('error', reject);
      if ([301, 302, 303, 307, 308].includes(response.statusCode) && redirects && response.headers.location) {
        response.resume();
        try {
          resolve(download(new URL(response.headers.location, url), limit, redirects - 1));
        } catch (error) {
          reject(error);
        }
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`Download failed (HTTP ${response.statusCode}): ${url.pathname}`));
        return;
      }
      const chunks = [];
      let size = 0;
      response.on('data', (chunk) => {
        size += chunk.length;
        if (size > limit) response.destroy(new Error('Release download exceeds size limit'));
        else chunks.push(chunk);
      });
      response.on('end', () => resolve(Buffer.concat(chunks)));
    });
    request.setTimeout(30_000, () => request.destroy(new Error('Release download timed out')));
    request.on('error', reject);
  });
}

function extractTar(archive, name) {
  // The release tool writes the binary as the first regular USTAR entry.
  const tar = zlib.gunzipSync(archive, { maxOutputLength: 12_000_000 });
  const entry = tar.toString('ascii', 0, 100).split('\0')[0];
  const size = Number.parseInt(tar.toString('ascii', 124, 136), 8);
  if (entry !== name || tar[156] !== 48 || !(size > 0 && size <= tar.length - 512)) {
    throw new Error('Release tarball has no binary in its first entry');
  }
  return tar.subarray(512, 512 + size);
}

function extractZip(zip, name) {
  // The release tool writes the deflated binary first, with no ZIP comment.
  const end = zip.length - 22;
  if (end < 0 || zip.readUInt32LE(end) !== 0x06054b50 || zip.readUInt32LE(0) !== 0x04034b50) {
    throw new Error('Invalid release ZIP');
  }
  const directory = zip.readUInt32LE(end + 16);
  const nameLength = zip.readUInt16LE(26);
  const start = 30 + nameLength + zip.readUInt16LE(28);
  if (directory < start || directory + 46 > end || zip.readUInt32LE(directory) !== 0x02014b50 ||
      zip.readUInt32LE(directory + 42) !== 0 || zip.readUInt16LE(8) !== 8 ||
      zip.toString('ascii', 30, 30 + nameLength) !== name) {
    throw new Error('Release ZIP has no binary in its first entry');
  }
  const size = zip.readUInt32LE(directory + 20);
  if (size === 0 || size > directory - start) throw new Error('Invalid binary size in release ZIP');
  return zlib.inflateRawSync(zip.subarray(start, start + size), { maxOutputLength: 10_000_000 });
}

async function install(directory, binary, name, platform, arch) {
  const windows = platform === 'windows';
  const asset = `agents-view_${version}_${platform}_${arch}.${windows ? 'zip' : 'tar.gz'}`;
  const base = (mirror || `https://github.com/wlkr-si/agents-view/releases/download/v${version}`).replace(/\/+$/, '');
  const sums = await download(new URL(`${base}/checksums.txt`), 64 * 1024);
  const expected = sums.toString('utf8').split(/\r?\n/)
    .map((line) => line.trim().split(/\s+/))
    .filter(([hash, file]) => file === asset && /^[a-f0-9]{64}$/.test(hash));
  if (expected.length !== 1) throw new Error(`Missing or ambiguous SHA-256 checksum for ${asset}`);

  const archive = await download(new URL(`${base}/${asset}`), 12_000_000);
  if (createHash('sha256').update(archive).digest('hex') !== expected[0][0]) {
    throw new Error(`SHA-256 checksum mismatch for ${asset}`);
  }
  const data = windows ? extractZip(archive, name) : extractTar(archive, name);

  fs.mkdirSync(directory, { recursive: true });
  const staging = fs.mkdtempSync(path.join(directory, 'install-'));
  try {
    const staged = path.join(staging, name);
    fs.writeFileSync(staged, data, { mode: 0o755 });
    // Another invocation can finish first. Both verified the same release.
    try {
      fs.linkSync(staged, binary);
    } catch (error) {
      if (error.code !== 'EEXIST') throw error;
    }
  } finally {
    fs.rmSync(staging, { recursive: true, force: true });
  }
  console.error(`Installed ${asset} (SHA-256 verified)`);
}

async function main() {
  const platform = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[process.platform];
  const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  if (!platform || !arch) throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);

  const name = platform === 'windows' ? 'agents-view.exe' : 'agents-view';
  const directory = path.join(__dirname, '..', '.cache', version);
  const binary = path.join(directory, name);
  if (!fs.existsSync(binary)) await install(directory, binary, name, platform, arch);

  const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.signal) process.kill(process.pid, result.signal);
  process.exitCode = result.status ?? 1;
}

main().catch((error) => {
  console.error(`agents-view: ${error.message}`);
  process.exitCode = 1;
});
