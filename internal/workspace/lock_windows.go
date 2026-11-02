//go:build windows

package workspace

// processAlive cannot portably probe an arbitrary process on Windows without
// extra permissions. Treat an existing lock as live and rely on the age
// threshold plus --force-lock instead.
func processAlive(pid int) bool { return pid != 0 }
