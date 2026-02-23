/**
 * Copies only the Shoelace icons used by the app into public/shoelace/assets/icons/.
 *
 * This avoids bundling the full 2000+ icon set (8MB+) while ensuring icons
 * load correctly when served by the Go server (which doesn't serve node_modules).
 *
 * To add a new icon: add its Bootstrap Icons name to the USED_ICONS array below,
 * then run `npm run copy:shoelace-icons`.
 */

import { cpSync, mkdirSync, existsSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');

const SRC = resolve(ROOT, 'node_modules/@shoelace-style/shoelace/dist/assets/icons');
const DEST = resolve(ROOT, 'public/shoelace/assets/icons');

/** Icons referenced via <sl-icon name="..."> across the app. */
const USED_ICONS = [
  'arrow-clockwise',
  'arrow-down-circle',
  'arrow-left',
  'arrow-repeat',
  'bell',
  'box-arrow-in-right',
  'box-arrow-right',
  'check-circle',
  'circle',
  'circle-fill',
  'clock',
  'clock-history',
  'code-square',
  'cpu',
  'emoji-frown',
  'exclamation-octagon',
  'exclamation-triangle',
  'file-text',
  'folder',
  'folder-fill',
  'folder2-open',
  'gear',
  'hdd-rack',
  'house',
  'hourglass-split',
  'list',
  'moon',
  'person',
  'play-circle',
  'plus-circle',
  'plus-lg',
  'question-circle',
  'shield-x',
  'stop-circle',
  'sun',
  'terminal',
  'trash',
  'x-circle',
];

mkdirSync(DEST, { recursive: true });

let copied = 0;
let missing = 0;

for (const name of USED_ICONS) {
  const src = resolve(SRC, `${name}.svg`);
  const dest = resolve(DEST, `${name}.svg`);
  if (existsSync(src)) {
    cpSync(src, dest);
    copied++;
  } else {
    console.warn(`  warning: icon "${name}.svg" not found in Shoelace assets`);
    missing++;
  }
}

console.log(`Shoelace icons: ${copied} copied${missing ? `, ${missing} missing` : ''}`);
