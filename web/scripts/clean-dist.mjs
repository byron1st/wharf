// Empties dist but keeps .gitkeep, which lets go:embed build before the first
// web build; vite's own emptyOutDir would delete it.
import { mkdirSync, readdirSync, rmSync } from 'node:fs'

const dist = new URL('../dist/', import.meta.url)
mkdirSync(dist, { recursive: true })
for (const name of readdirSync(dist)) {
  if (name !== '.gitkeep') rmSync(new URL(name, dist), { recursive: true, force: true })
}
