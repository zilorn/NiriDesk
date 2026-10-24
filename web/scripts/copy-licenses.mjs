import { copyFile, mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = resolve(root, "node_modules/@novnc/novnc");
const target = resolve(root, "dist/licenses");

await mkdir(target, { recursive: true });

const files = [
  ["LICENSE.txt", "novnc-LICENSE.txt"],
  ["AUTHORS", "novnc-AUTHORS"],
  ["docs/LICENSE.MPL-2.0", "LICENSE.MPL-2.0"],
  ["docs/LICENSE.BSD-2-Clause", "LICENSE.BSD-2-Clause"],
  ["docs/LICENSE.BSD-3-Clause", "LICENSE.BSD-3-Clause"],
  ["docs/LICENSE.OFL-1.1", "LICENSE.OFL-1.1"],
  ["vendor/pako/LICENSE", "pako-LICENSE"],
];

for (const [from, to] of files) {
  await copyFile(resolve(source, from), resolve(target, to));
}
