import { cp, mkdir, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const source = path.join(root, "node_modules", "@novnc", "novnc");
const target = path.join(root, "web", "static", "vendor", "novnc");

await rm(target, { recursive: true, force: true });
await mkdir(path.dirname(target), { recursive: true });
await cp(path.join(source, "lib"), path.join(target, "lib"), { recursive: true });
process.stdout.write("Vendored noVNC browser modules into web/static/vendor/novnc\n");
