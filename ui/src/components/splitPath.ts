// splitPath breaks a full path into the parent listing *prefix* (everything up
// to and including the last "/", or "" when there is none) and the trailing
// segment being typed. The prefix is what we ask the backend to list; the tail
// is what we filter that listing by. Splitting on the last "/" preserves any
// leading slash so completing a segment keeps the path shape intact.
export function splitPath(full: string): { prefix: string; tail: string } {
  const idx = full.lastIndexOf("/")
  if (idx < 0) return { prefix: "", tail: full }
  return { prefix: full.slice(0, idx + 1), tail: full.slice(idx + 1) }
}
