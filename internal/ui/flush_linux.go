package ui

import (
	"os"
	"syscall"
)

// tcflsh is the TCFLSH ioctl of x86 and arm Linux, which the syscall package does not name.
const tcflsh = 0x540B

// flushInput drops the bytes waiting in the terminal's input queue (tcflush with
// TCIFLUSH). Not a terminal: nothing happens.
func flushInput(f *os.File) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tcflsh, syscall.TCIFLUSH)
}
