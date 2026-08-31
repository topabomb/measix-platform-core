#!/usr/bin/env node
// scripts/dev-setup.mjs — one-shot local development bootstrap for measix S0.
//
// Generates cryptographic material, applies the published migration SQL,
// and bootstraps the initial administrator. All secrets land in .secrets/
// (gitignored). Run:  npm run setup
import { randomBytes } from "node:crypto";
import { writeFileSync, mkdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..");
const SECRETS = join(ROOT, ".secrets");
const DATA = join(ROOT, ".data");
const BACKEND = join(ROOT, "backend");

function run(cmd, args, opts = {}) {
  console.log(`$ ${cmd} ${args.join(" ")}`);
  const result = spawnSync(cmd, args, { stdio: "inherit", ...opts });
  if (result.status !== 0) {
    console.error(`Command failed: ${cmd} ${args.join(" ")} (exit ${result.status})`);
    process.exit(result.status ?? 1);
  }
}

function writeSecret(name, bytes) {
  const path = join(SECRETS, name);
  writeFileSync(path, bytes, { mode: 0o600, flag: "wx" });
  console.log(`  generated ${name} (${bytes.length} bytes)`);
}

console.log("=== measix S0 dev setup ===\n");

// 1. Create directories
mkdirSync(SECRETS, { recursive: true });
mkdirSync(DATA, { recursive: true });

// 2. Generate cryptographic material (idempotent — skip if exists)
console.log("[1/4] Generating cryptographic material...");
const masterKeyPath = join(SECRETS, "master.key");
const jwtKeyPath = join(SECRETS, "jwt-ed25519.seed");
const relayTokenPath = join(SECRETS, "relay-service.token");

if (!existsSync(masterKeyPath)) {
  writeSecret("master.key", randomBytes(32)); // AES-256 master key
} else { console.log("  master.key exists, skipping"); }

if (!existsSync(jwtKeyPath)) {
  writeSecret("jwt-ed25519.seed", randomBytes(32)); // Ed25519 seed (32 bytes)
} else { console.log("  jwt-ed25519.seed exists, skipping"); }

if (!existsSync(relayTokenPath)) {
  const token = randomBytes(32).toString("base64url");
  writeFileSync(relayTokenPath, token + "\n", { mode: 0o600, flag: "wx" });
  console.log("  generated relay-service.token");
} else { console.log("  relay-service.token exists, skipping"); }

// 3. Apply checked, ordered development migrations; never silently adopt an unknown schema.
console.log("\n[2/4] Applying migration to local hub.db...");
const hubDBRel = "../.data/hub.db";
const masterKeyRel = "../.secrets/master.key";
const jwtKeyRel = "../.secrets/jwt-ed25519.seed";
const adminPwRel = "../.secrets/admin-password.txt";
run("go", ["run", "./cmd/devmigrate", "--db", hubDBRel], { cwd: BACKEND });

// 4. Bootstrap admin user
console.log("\n[3/4] Bootstrapping admin user...");
const adminPwPath = join(SECRETS, "admin-password.txt");
if (!existsSync(adminPwPath)) {
  const pw = "measix-local-" + randomBytes(4).toString("hex");
  writeFileSync(adminPwPath, pw + "\n", { mode: 0o600, flag: "wx" });
  console.log("  generated admin-password.txt");
}

const bootstrapArgs = [
  "run", "./cmd/control-hub", "bootstrap-admin", "--if-empty",
  "--db", hubDBRel,
  "--master-key-file", masterKeyRel,
  "--jwt-private-key-file", jwtKeyRel,
  "--deployment-name", "MEASIX-LOCAL",
  "--username", "admin",
  "--display-name", "Local Administrator",
  "--password-file", adminPwRel,
];
run("go", bootstrapArgs, { cwd: BACKEND });

// 5. Done — print next steps
console.log("\n[4/4] Setup complete!\n");
console.log("Secrets in:  .secrets/");
console.log("Data in:     .data/");
console.log("");
console.log("Admin credentials:");
console.log("  username: admin");
console.log("  initial password: see .secrets/admin-password.txt (not reset on repeat setup)");
console.log("");
console.log("Next: npm start   (starts hub + relay + console)");
console.log("     npm run start:hub    (control-hub only)");
console.log("     npm run start:relay  (runtime-relay only)");
console.log("     npm run start:console (Quasar dev only)");
