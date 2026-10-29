//go:build unix

package exec

import (
	"os"
	"syscall"
)

// pollableTTY returns a non-blocking duplicate of the PTY slave for the
// session's own reads and writes.
//
// The SDK hands over a slave in blocking mode (term.MakeRaw calls Fd(), as
// does creack/pty's ioctl for the master). Closing a blocking *os.File
// does not interrupt a Read on it: the close(2) waits until the Read
// returns. A Read on the slave only returns on input, so the manager's
// Terminal.Close would leave the slave open, the manager's master Read
// would never see the hangup, and CloseSession would time out. A
// non-blocking duplicate lives in Go's poller, where Close interrupts a
// pending Read at once.
func pollableTTY(tty *os.File) (*os.File, error) {
	raw, err := tty.SyscallConn()
	if err != nil {
		return nil, err
	}
	fd, dupErr := -1, error(nil)
	if err := raw.Control(func(f uintptr) {
		syscall.ForkLock.RLock()
		defer syscall.ForkLock.RUnlock()
		if fd, dupErr = syscall.Dup(int(f)); dupErr == nil {
			syscall.CloseOnExec(fd)
		}
	}); err != nil {
		return nil, err
	}
	if dupErr != nil {
		return nil, dupErr
	}
	// O_NONBLOCK belongs to the open file description, which the manager's
	// slave shares; the manager never reads or writes the slave itself.
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), tty.Name()), nil
}
