// Stamp a release version into the three desktop-app manifests so the bundle
// version matches the published tag. Run from the repo root:
//   node .github/scripts/set-version.mjs 1.2.3
import { readFileSync, writeFileSync } from 'node:fs';

const version = process.argv[2];
if (!version) {
  console.error('usage: set-version.mjs <version>');
  process.exit(1);
}

function setJson(path, key) {
  const obj = JSON.parse(readFileSync(path, 'utf8'));
  obj[key] = version;
  writeFileSync(path, JSON.stringify(obj, null, 2) + '\n');
}

setJson('apps/desktop/package.json', 'version');
setJson('apps/desktop/src-tauri/tauri.conf.json', 'version');

const cargoPath = 'apps/desktop/src-tauri/Cargo.toml';
const cargo = readFileSync(cargoPath, 'utf8');
// Replace only the [package] version (first `version = "..."` at column 0).
writeFileSync(cargoPath, cargo.replace(/^version = ".*"/m, `version = "${version}"`));

console.log(`stamped version ${version}`);
