# 14 — npm, package.json, semver, Monorepos

> **Goal:** Manage dependencies, scripts, and project structure professionally — including semver, lockfiles, security audits, and monorepo basics.

---

## 1. package.json — Mental Model

`package.json` is the manifest. It declares your project's name, dependencies, scripts, and how it's exposed to consumers.

```json
{
  "name": "my-app",
  "version": "1.4.2",
  "type": "module",
  "main": "./dist/index.js",
  "exports": {
    ".": {
      "import": "./dist/index.js",
      "types": "./dist/index.d.ts"
    },
    "./cli": "./dist/cli.js"
  },
  "bin": { "my-tool": "./dist/cli.js" },
  "files": ["dist", "README.md"],
  "engines": { "node": ">=20" },
  "scripts": {
    "dev": "vite",
    "build": "tsc -b",
    "test": "vitest",
    "lint": "eslint .",
    "format": "prettier --write ."
  },
  "dependencies": { "react": "^18.3.0" },
  "devDependencies": { "vitest": "^2.0.0", "typescript": "^5.5.0" },
  "peerDependencies": { "react": ">=18" },
  "optionalDependencies": { "fsevents": "^2.3.0" }
}
```

Key fields:
- **`type`** — `"module"` for ESM, otherwise CJS.
- **`main` / `exports`** — entry points for consumers. `exports` is modern and exact.
- **`bin`** — exposes commands when installed globally or via `npx`.
- **`files`** — what gets published (tighten this; default publishes too much).
- **`engines`** — minimum runtime versions.
- **`scripts`** — runnable via `npm run <name>`.

### Dependency types
- **`dependencies`** — required at runtime.
- **`devDependencies`** — only needed during development (test, build, lint).
- **`peerDependencies`** — required, but you expect the host app to provide it (plugins).
- **`optionalDependencies`** — install if possible; skip silently if not.

---

## 2. Semver & Lockfiles — Under the Hood

### Semantic Versioning
`MAJOR.MINOR.PATCH` — e.g. `2.4.7`.
- **MAJOR** — breaking changes (incompatible API).
- **MINOR** — new features, backward compatible.
- **PATCH** — bug fixes only.

Pre-release: `2.0.0-rc.1`, `2.0.0-beta.3`. Build metadata: `2.0.0+build.42`.

### Range syntax in package.json
```
"^1.4.2"  → >=1.4.2 <2.0.0     (caret — most common; allow any minor/patch)
"~1.4.2"  → >=1.4.2 <1.5.0     (tilde — allow patch only)
"1.4.2"   → exact
">=1.4.2 <2"  → explicit range
"latest"  → always newest (DO NOT — non-reproducible)
"git+https://github.com/x/y#main" → git URL
```

`^0.x.y` is special: caret on a 0.x version allows only patch changes (because pre-1.0 anything can break).

### Lockfiles — reproducible installs
- npm → `package-lock.json`
- pnpm → `pnpm-lock.yaml`
- yarn → `yarn.lock`
- bun → `bun.lock` (text format, human-diffable)

The lockfile records **exact** versions of every package in the tree. **Always commit it.** It's why "works on my machine" disagreements are so much rarer in 2026.

`npm install` updates the lockfile; `npm ci` installs *strictly* from it (faster, used in CI).

### npm vs pnpm vs yarn vs bun

| | Speed | Disk | Notes |
|---|------|------|-------|
| **npm** | OK | duplicates per project | the default, ships with Node |
| **pnpm** | fast | hard-links to global store, dedup | strict by default; great for monorepos |
| **yarn** (Berry) | fast | PnP option | own ecosystem; Plug'n'Play is divisive |
| **bun** | fastest | similar to npm | also a runtime; very fast installs |

If you're starting fresh, **pnpm** is the safest "modern" pick. Speed close to bun, fewer surprises than yarn.

---

## 3. Workflows: install, update, audit, publish

### Install
```bash
npm install                  # everything from package.json + lockfile
npm install lodash           # add to dependencies
npm install -D vitest        # add to devDependencies
npm install -g typescript    # global (avoid for project tooling)
npm ci                       # clean install from lockfile (CI)
```

### Update
```bash
npm outdated                 # see what's old
npm update                   # update within ranges
npm install lodash@latest    # bump to newest, update package.json
npx npm-check-updates -u     # rewrite all ranges to latest
```

### Audit & security
```bash
npm audit                    # report vulnerabilities
npm audit fix                # auto-bump to safe versions (if possible)
npm audit fix --force        # may update across major versions — review!
```
Set up Dependabot or Renovate for automated PRs.

### Publishing
```bash
npm version patch            # bump version + git tag
npm publish                  # publish to npm registry
npm publish --access public  # required for first publish of a scoped package
npm publish --tag beta       # tag a pre-release
```

### Running scripts
```bash
npm run build                # runs scripts.build
npm test                     # special: shortcut for "npm run test"
npm run                      # list all scripts
```
Scripts have `node_modules/.bin` on PATH automatically — that's how `vitest`, `eslint` work without globals.

`npx <pkg>` runs a binary, downloading temporarily if needed:
```bash
npx create-vite@latest my-app
```

### `package.json` patterns
```json
"scripts": {
  "build": "tsc -b",
  "dev": "vite",
  "test": "vitest",
  "test:ci": "vitest run --coverage",
  "lint": "eslint .",
  "format": "prettier --write .",
  "prepare": "husky",                // runs after install — for git hooks
  "prepublishOnly": "npm run build && npm test"  // safety gate
}
```

---

## 4. Practical Application — Monorepos

A **monorepo** holds multiple packages in one repo with shared tooling. Common in real orgs (Google, FB, Vercel, Stripe).

### Workspaces — built-in
`package.json` at the root:
```json
{
  "name": "my-monorepo",
  "private": true,
  "workspaces": ["packages/*", "apps/*"]
}
```
Layout:
```
my-monorepo/
  package.json           // root
  package-lock.json
  packages/
    ui/
      package.json       // name: "@acme/ui"
    utils/
      package.json       // name: "@acme/utils"
  apps/
    web/
      package.json       // depends on @acme/ui, @acme/utils
```

Internal dependency:
```json
// apps/web/package.json
{
  "dependencies": {
    "@acme/ui": "workspace:*",
    "@acme/utils": "workspace:*"
  }
}
```

`workspace:*` (pnpm/yarn/bun) means "use the local workspace version." With npm, just use `*` or a real range; npm hoists symlinks automatically.

Run a script in one workspace:
```bash
npm run build -w @acme/ui
pnpm --filter @acme/ui build
```
Run across all:
```bash
npm run build --workspaces
pnpm -r build
```

### Tooling layered on top
- **Turborepo** — caches script outputs across machines/CI; massive speedup.
- **Nx** — monorepo orchestration with code generation and dependency graph.
- **Changesets** — versioning + changelogs for multi-package publishing.

A typical pnpm + Turborepo setup is a strong default in 2026.

### Real example: changesets workflow
```bash
pnpm changeset                 # create a changeset (interactive)
git commit -am "feat: x"
# CI later runs:
pnpm changeset version         # bump versions, write CHANGELOGs
pnpm changeset publish         # publish to npm
```

---

## 5. Common Mistakes & Gotchas

- **Not committing the lockfile.** Suddenly different machines pull different versions; bugs are unreproducible. Commit it.
- **Range too loose:** `"latest"` or no version → broken builds when upstream changes.
- **`npm install` in CI** instead of `npm ci`. The former mutates the lockfile and is slower.
- **Globally installed dev tools.** Avoid `npm install -g typescript` for projects — you'll be on the wrong version somewhere. Use `npx` or `devDependencies`.
- **Not pinning Node version.** Add `engines.node` AND a `.nvmrc` / `.node-version` file. CI should match.
- **Publishing too much:** Without `files`, you publish your whole repo including `.env`, tests, source maps. Always set `files`.
- **Missing `peerDependency`:** library installs but breaks at runtime because the host doesn't have React.
- **Accidentally breaking semver:** removing an exported function in a minor release breaks consumers. Use `--dry-run` and review the diff.
- **`postinstall` scripts as a malware vector.** They run automatically on install. Audit dependencies, especially deep transitive ones.
- **`node_modules` in git.** Never. Add `.gitignore`.
- **Mixing package managers** (some commits use yarn, others npm) → conflicting lockfiles. Pick one per repo, document it.

```js
// "Wat"
"^0.1.5"  // allows 0.1.5 to <0.2.0 — NOT to <1.0.0
"^1.0.0-beta.1" // allows 1.0.0-beta.1 to <2.0.0 only — pre-release ranges have surprises
```

### `npm`/`node` version manager hygiene
```bash
# .nvmrc
22

# .npmrc — useful in monorepos / CI
engine-strict=true        # fail install if engines.node mismatches
save-exact=true           # write exact versions (no ^), if you want determinism
```

---

## 🎯 Key Takeaways

- **Commit your lockfile.** Always. `npm ci` (or `pnpm install --frozen-lockfile`) in CI.
- **Use `^` for libraries you trust, exact for things you don't.** `0.x` packages need extra care.
- **Pin Node** with `engines` + `.nvmrc`. Mismatched runtimes are a top source of "works on my machine."
- **Monorepos with workspaces** are the modern default for multi-package projects. pnpm + Turborepo is a strong stack.
- **Treat `package.json` as production config.** `files`, `exports`, `engines`, `scripts` — get these right and your library/app behaves predictably for consumers and CI.

---

*← [13 Node.js Essentials](./13_node_essentials.md) | [next → 15 Testing](./15_testing.md)*
