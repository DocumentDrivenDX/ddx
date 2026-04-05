import initSqlJs, { type Database } from 'sql.js'
import type { Bead } from '../types'

let db: Database | null = null
let sqlPromise: ReturnType<typeof initSqlJs> | null = null

const SCHEMA = `
CREATE TABLE IF NOT EXISTS beads (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  issue_type TEXT,
  priority INTEGER DEFAULT 2,
  owner TEXT,
  description TEXT,
  acceptance TEXT,
  spec_id TEXT,
  labels TEXT,
  parent TEXT,
  created_at TEXT,
  updated_at TEXT,
  raw_json TEXT
);

CREATE TABLE IF NOT EXISTS bead_deps (
  bead_id TEXT NOT NULL,
  dep_id TEXT NOT NULL,
  PRIMARY KEY (bead_id, dep_id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS beads_fts USING fts5(
  id, title, description, acceptance, labels,
  content='beads', content_rowid='rowid'
);

CREATE INDEX IF NOT EXISTS idx_beads_status ON beads(status);
CREATE INDEX IF NOT EXISTS idx_beads_priority ON beads(priority);
CREATE INDEX IF NOT EXISTS idx_beads_spec_id ON beads(spec_id);
`

async function getSQL() {
  if (!sqlPromise) {
    sqlPromise = initSqlJs({
      locateFile: (file: string) => `https://sql.js.org/dist/${file}`,
    })
  }
  return sqlPromise
}

export async function initDb(): Promise<Database> {
  if (db) return db
  const SQL = await getSQL()
  db = new SQL.Database()
  db.run(SCHEMA)
  return db
}

export async function loadBeads(beads: Bead[]): Promise<void> {
  const d = await initDb()

  d.run('DELETE FROM bead_deps')
  d.run('DELETE FROM beads')

  const insertBead = d.prepare(
    `INSERT OR REPLACE INTO beads (id, title, status, issue_type, priority, owner, description, acceptance, spec_id, labels, parent, created_at, updated_at, raw_json)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
  )

  const insertDep = d.prepare(
    'INSERT OR REPLACE INTO bead_deps (bead_id, dep_id) VALUES (?, ?)'
  )

  for (const b of beads) {
    const specId = (b as any).spec_id ?? (b as any)['spec-id'] ?? ''
    insertBead.run([
      b.id,
      b.title,
      b.status,
      b.issue_type ?? 'task',
      b.priority ?? 2,
      b.owner ?? '',
      b.description ?? '',
      b.acceptance ?? '',
      specId,
      (b.labels ?? []).join(','),
      (b as any).parent ?? '',
      b.created_at ?? '',
      b.updated_at ?? '',
      JSON.stringify(b),
    ])

    for (const dep of b.dependencies ?? []) {
      if (dep.depends_on_id) {
        insertDep.run([b.id, dep.depends_on_id])
      }
    }
  }

  insertBead.free()
  insertDep.free()

  // Rebuild FTS index
  d.run(`INSERT INTO beads_fts(beads_fts) VALUES('rebuild')`)
}

export function queryBeadsByStatus(status: string): Bead[] {
  if (!db) return []
  const rows = db.exec(
    'SELECT raw_json FROM beads WHERE status = ? ORDER BY priority ASC, created_at ASC',
    [status]
  )
  return parseRows(rows)
}

export function queryAllBeads(): Bead[] {
  if (!db) return []
  const rows = db.exec('SELECT raw_json FROM beads ORDER BY priority ASC, created_at ASC')
  return parseRows(rows)
}

export function searchBeads(query: string): Bead[] {
  if (!db || !query.trim()) return queryAllBeads()
  const rows = db.exec(
    `SELECT b.raw_json FROM beads_fts fts
     JOIN beads b ON b.rowid = fts.rowid
     WHERE beads_fts MATCH ?
     ORDER BY rank`,
    [query + '*']
  )
  return parseRows(rows)
}

export function queryReadyBeads(): Bead[] {
  if (!db) return []
  const rows = db.exec(
    `SELECT b.raw_json FROM beads b
     WHERE b.status = 'open'
     AND NOT EXISTS (
       SELECT 1 FROM bead_deps bd
       JOIN beads dep ON bd.dep_id = dep.id
       WHERE bd.bead_id = b.id AND dep.status != 'closed'
     )
     ORDER BY b.priority ASC, b.created_at ASC`
  )
  return parseRows(rows)
}

export function queryDependencies(beadId: string): Bead[] {
  if (!db) return []
  const rows = db.exec(
    `WITH RECURSIVE deps AS (
       SELECT dep_id FROM bead_deps WHERE bead_id = ?
       UNION
       SELECT bd.dep_id FROM bead_deps bd
       JOIN deps d ON bd.bead_id = d.dep_id
     )
     SELECT b.raw_json FROM beads b JOIN deps d ON b.id = d.dep_id`,
    [beadId]
  )
  return parseRows(rows)
}

export function queryStatusCounts(): Record<string, number> {
  if (!db) return {}
  const rows = db.exec('SELECT status, COUNT(*) as cnt FROM beads GROUP BY status')
  const counts: Record<string, number> = {}
  if (rows.length > 0) {
    for (const row of rows[0].values) {
      counts[row[0] as string] = row[1] as number
    }
  }
  return counts
}

function parseRows(rows: ReturnType<Database['exec']>): Bead[] {
  if (rows.length === 0) return []
  return rows[0].values.map((row: (string | number | Uint8Array | null)[]) => JSON.parse(row[0] as string) as Bead)
}
