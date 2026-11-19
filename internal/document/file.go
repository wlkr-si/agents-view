package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

// DecodeUTF8 uses the same replacement boundaries as Node's UTF-8 decoding.
func DecodeUTF8(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	var text strings.Builder
	for len(raw) > 0 {
		char, size := utf8.DecodeRune(raw)
		if char == utf8.RuneError && size == 1 && raw[0] >= 0xc2 && raw[0] <= 0xf4 {
			width := 2
			if raw[0] >= 0xf0 {
				width = 4
			} else if raw[0] >= 0xe0 {
				width = 3
			}
			for size < min(len(raw), width) && raw[size]&0xc0 == 0x80 {
				if size == 1 && ((raw[0] == 0xe0 && raw[1] < 0xa0) ||
					(raw[0] == 0xed && raw[1] >= 0xa0) ||
					(raw[0] == 0xf0 && raw[1] < 0x90) ||
					(raw[0] == 0xf4 && raw[1] >= 0x90)) {
					break
				}
				size++
			}
		}
		text.WriteRune(char)
		raw = raw[size:]
	}
	return text.String()
}

func ReadFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", FileError("open", err)
	}
	return DecodeUTF8(raw), nil
}

func ReadBody() (string, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", FileError("read", err)
	}
	return DecodeUTF8(raw), nil
}

var fileErrors = []struct {
	err           syscall.Errno
	code, message string
}{
	{syscall.ENOENT, "ENOENT", "no such file or directory"},
	{syscall.ENOTDIR, "ENOTDIR", "not a directory"},
	{syscall.EISDIR, "EISDIR", "illegal operation on a directory"},
	{syscall.EEXIST, "EEXIST", "file already exists"},
	{syscall.EACCES, "EACCES", "permission denied"},
	{syscall.EPERM, "EPERM", "operation not permitted"},
	{syscall.EROFS, "EROFS", "read-only file system"},
	{syscall.ENOSPC, "ENOSPC", "no space left on device"},
	{syscall.ELOOP, "ELOOP", "too many symbolic links encountered"},
	{syscall.ENAMETOOLONG, "ENAMETOOLONG", "name too long"},
	{syscall.EBADF, "EBADF", "bad file descriptor"},
	{syscall.EMFILE, "EMFILE", "too many open files"},
}

func FileError(operation string, err error) error {
	path := ""
	var file *os.PathError
	var link *os.LinkError
	switch {
	case errors.As(err, &file):
		if file.Op != "open" {
			operation = file.Op
		}
		if operation != "read" && operation != "write" && operation != "close" {
			path = " '" + file.Path + "'"
		}
	case errors.As(err, &link):
		operation = link.Op
		path = " '" + link.Old + "' -> '" + link.New + "'"
	}
	for _, entry := range fileErrors {
		if errors.Is(err, entry.err) {
			return fmt.Errorf("%s: %s, %s%s", entry.code, entry.message, operation, path)
		}
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return fmt.Errorf("UNKNOWN: %s, %s%s", errno.Error(), operation, path)
	}
	return fmt.Errorf("%s: %s%s", operation, strconv.Quote(err.Error()), path)
}
