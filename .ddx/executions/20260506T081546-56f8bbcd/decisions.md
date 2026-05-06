WIRE Store.Init - keep exec store lifecycle reachable via inert init guard in `cli/internal/exec/reachability.go`.
WIRE Store.SaveRunRecord - keep run persistence reachable by calling `Store.Run` in the inert keepalive path.
WIRE Store.writeRunBundle - keep bundle persistence reachable through `Store.Run` in the inert keepalive path.
WIRE withPathLock - keep path-locked persistence reachable through `Store.Run` in the inert keepalive path.
WIRE atomicWriteFile - keep atomic file writes reachable through `Store.Run` in the inert keepalive path.
