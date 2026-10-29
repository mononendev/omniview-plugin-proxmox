//go:build !unix

package exec

import (
	"errors"
	"os"
)

// pollableTTY is unavailable without Unix PTYs; the SDK cannot open one
// there either, so no session reaches the handler.
func pollableTTY(*os.File) (*os.File, error) {
	return nil, errors.New("shells need a Unix PTY")
}
