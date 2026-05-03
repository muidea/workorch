package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

type readline struct {
	history  []string
	hpos     int
	oldState *termState
	activeFD int
	rawBuf   []byte // accumulates partial UTF-8 bytes in raw mode
}

type termState struct {
	termios syscall.Termios
}

func newReadline() *readline {
	return &readline{history: make([]string, 0, 64), hpos: -1}
}

func (r *readline) addHistory(line string) {
	if line == "" {
		return
	}
	if len(r.history) > 0 && r.history[len(r.history)-1] == line {
		return
	}
	r.history = append(r.history, line)
	r.hpos = len(r.history)
}

func (r *readline) makeRaw() error {
	fd := int(os.Stdin.Fd())
	r.activeFD = fd
	var old syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&old)), 0, 0, 0); err != 0 {
		return fmt.Errorf("tcgetattr: %v", err)
	}
	r.oldState = &termState{termios: old}

	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&raw)), 0, 0, 0); err != 0 {
		return fmt.Errorf("tcsetattr: %v", err)
	}
	return nil
}

func (r *readline) restore() {
	if r.oldState == nil {
		return
	}
	syscall.Syscall6(syscall.SYS_IOCTL, uintptr(r.activeFD), syscall.TCSETS, uintptr(unsafe.Pointer(&r.oldState.termios)), 0, 0, 0)
	r.oldState = nil
}

var currentPrompt string

func (r *readline) ReadLine(prompt string) (string, error) {
	currentPrompt = prompt
	if err := r.makeRaw(); err != nil {
		fmt.Fprint(os.Stderr, prompt)
		return r.readLinePlain()
	}
	defer r.restore()
	fmt.Fprint(os.Stderr, prompt)
	return r.readLineRaw()
}

func (r *readline) readLinePlain() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)
	r.addHistory(line)
	return line, nil
}

// readUTF8Char reads a single complete UTF-8 character from stdin (raw mode).
// Returns ESC (0x1b) immediately without accumulating more bytes.
func (r *readline) readUTF8Char() (rune, error) {
	b := make([]byte, 1)
	if _, err := os.Stdin.Read(b); err != nil {
		return 0, err
	}
	if b[0] == 0x1b || b[0] < 0x80 {
		return rune(b[0]), nil // ASCII or ESC — single byte
	}
	r.rawBuf = []byte{b[0]}
	for {
		if _, err := os.Stdin.Read(b); err != nil {
			return 0, err
		}
		r.rawBuf = append(r.rawBuf, b[0])
		if utf8.FullRune(r.rawBuf) {
			ch, _ := utf8.DecodeRune(r.rawBuf)
			return ch, nil
		}
	}
}

func (r *readline) readLineRaw() (string, error) {
	var line []rune
	pos := 0

	for {
		ch, err := r.readUTF8Char()
		if err != nil {
			return "", err
		}

		switch {
		case ch == 0x0d: // Enter
			fmt.Fprintf(os.Stderr, "\r\033[K%s\r\n", string(line))
			result := string(line)
			r.addHistory(result)
			return result, nil

		case ch == 0x03: // Ctrl+C
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return "", fmt.Errorf("interrupted")

		case ch == 0x7f || ch == 0x08: // Backspace
			if pos > 0 {
				pos--
				line = append(line[:pos], line[pos+1:]...)
				r.redraw(line, pos)
			}

		case ch == 0x1b: // Escape
			// Check if there's a follow-up byte (arrow key sequence is ESC [ X)
			seq := make([]byte, 2)
			n, _ := os.Stdin.Read(seq)
			if n == 0 || seq[0] != '[' {
				// Plain ESC — cancel input
				fmt.Fprintf(os.Stderr, "\r\033[K%s\r\n", currentPrompt)
				return "", nil
			}
			switch seq[1] {
			case 'A': // Up
				if r.hpos > 0 {
					r.hpos--
					line = []rune(r.history[r.hpos])
					pos = len(line)
					r.redraw(line, pos)
				}
			case 'B': // Down
				if r.hpos < len(r.history)-1 {
					r.hpos++
					line = []rune(r.history[r.hpos])
				} else {
					r.hpos = len(r.history)
					line = nil
				}
				pos = len(line)
				r.redraw(line, pos)
			case 'C': // Right
				if pos < len(line) {
					pos++
					r.redraw(line, pos)
				}
			case 'D': // Left
				if pos > 0 {
					pos--
					r.redraw(line, pos)
				}
			case 'H': // Home
				pos = 0
				r.redraw(line, pos)
			case 'F': // End
				pos = len(line)
				r.redraw(line, pos)
			case '3': // Delete
				b := make([]byte, 1)
				os.Stdin.Read(b)
				if b[0] == '~' && pos < len(line) {
					line = append(line[:pos], line[pos+1:]...)
					r.redraw(line, pos)
				}
			}

		default:
			if ch >= 32 { // Printable (including all Unicode)
				line = append(line[:pos], append([]rune{ch}, line[pos:]...)...)
				pos++
				r.redraw(line, pos)
			}
		}
	}
}

func (r *readline) redraw(line []rune, pos int) {
	fmt.Fprintf(os.Stderr, "\r\033[K%s%s", currentPrompt, string(line))
	if pos < len(line) {
		fmt.Fprintf(os.Stderr, "\033[%dD", len(line)-pos)
	}
}
