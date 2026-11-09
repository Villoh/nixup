// Package exit holds the process exit codes shared by every command.
package exit

const (
	OK      = 0 // nothing to do, or done
	Pending = 1 // updates available (check), or a precondition refused the run (update)
	Error   = 2 // something failed
)
