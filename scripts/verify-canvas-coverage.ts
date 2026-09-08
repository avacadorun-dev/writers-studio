import { mkdir, readFile } from "node:fs/promises";

const minimum = 90;
const frontendSources = ["src/entities/canvas/", "src/features/canvas-autosave/"];

const lcov = await readFile("coverage/frontend/lcov.info", "utf8");
let frontendFound = 0;
let frontendLines = 0;
let frontendHit = 0;
for (const record of lcov.split("end_of_record")) {
  const source = record.match(/^SF:(.+)$/m)?.[1];
  if (!source || !frontendSources.some((prefix) => source.startsWith(prefix))) continue;
  const lines = Number(record.match(/^LF:(\d+)$/m)?.[1] ?? 0);
  const hit = Number(record.match(/^LH:(\d+)$/m)?.[1] ?? 0);
  frontendFound += 1;
  frontendLines += lines;
  frontendHit += hit;
}
if (frontendFound === 0 || frontendLines === 0) {
  throw new Error("Canvas frontend sources are missing from LCOV");
}

await mkdir("coverage/backend", { recursive: true });
const profile = "coverage/backend/canvas.out";
const goTest = Bun.spawnSync(["go", "test", `-coverprofile=${profile}`, "./internal/canvas"], {
  stdout: "inherit",
  stderr: "inherit",
});
if (goTest.exitCode !== 0) process.exit(goTest.exitCode);
const goCover = Bun.spawnSync(["go", "tool", "cover", `-func=${profile}`]);
if (goCover.exitCode !== 0) {
  process.stderr.write(goCover.stderr.toString());
  process.exit(goCover.exitCode);
}
const goOutput = goCover.stdout.toString();
const backend = Number(goOutput.match(/total:\s+\(statements\)\s+([\d.]+)%/)?.[1]);
const frontend = (frontendHit / frontendLines) * 100;

console.log(
  `Canvas frontend line coverage: ${frontend.toFixed(2)}% (${frontendHit}/${frontendLines})`,
);
console.log(`Canvas backend statement coverage: ${backend.toFixed(2)}%`);
if (!Number.isFinite(backend) || frontend < minimum || backend < minimum) {
  throw new Error(`Canvas coverage must be at least ${minimum}%`);
}
