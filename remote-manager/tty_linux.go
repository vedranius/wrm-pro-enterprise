//go:build linux && (amd64 || arm64 || arm || 386)

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// Pseudo terminals and serial ports on Linux, with the syscall package only.

// termios bits the syscall package does not define (same on x86, arm and arm64).
const (
	termCBAUD   = 0x100f
	termCRTSCTS = 0x80000000
)

// ioctlFile runs an ioctl without switching the file to blocking mode (unlike f.Fd()),
// so reads stay interruptible by Close.
func ioctlFile(f *os.File, req uintptr, arg unsafe.Pointer) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if cerr := rc.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	}); cerr != nil {
		return cerr
	}
	if errno != 0 {
		return errno
	}
	return nil
}

// openPTYPair opens a new pseudo terminal: the master and the path of the slave.
func openPTYPair() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", fmt.Errorf("cannot open a pseudo terminal: %v", err)
	}
	var unlock int32
	var n uint32
	if err := ioctlFile(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, "", fmt.Errorf("pseudo terminal: %v", err)
	}
	if err := ioctlFile(master, syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil {
		master.Close()
		return nil, "", fmt.Errorf("pseudo terminal: %v", err)
	}
	return master, fmt.Sprintf("/dev/pts/%d", n), nil
}

// startInPTY starts cmd with a new pseudo terminal as its controlling terminal and
// returns the master side.
func startInPTY(cmd *exec.Cmd, cols, rows int) (*os.File, error) {
	master, path, err := openPTYPair()
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("pseudo terminal: %v", err)
	}
	setPTYSize(master, cols, rows)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		slave.Close()
		master.Close()
		return nil, err
	}
	slave.Close()
	return master, nil
}

func setPTYSize(f *os.File, cols, rows int) {
	ws := struct{ Row, Col, X, Y uint16 }{uint16(rows), uint16(cols), 0, 0}
	ioctlFile(f, syscall.TIOCSWINSZ, unsafe.Pointer(&ws))
}

var serialBauds = map[int]uint32{1200: syscall.B1200, 2400: syscall.B2400, 4800: syscall.B4800, 9600: syscall.B9600, 19200: syscall.B19200,
	38400: syscall.B38400, 57600: syscall.B57600, 115200: syscall.B115200, 230400: syscall.B230400, 460800: syscall.B460800, 921600: syscall.B921600}

// openSerialPort opens a serial device in raw mode with the given line settings.
func openSerialPort(path string, so serialOptions) (*os.File, error) {
	baud, ok := serialBauds[so.Baud]
	if !ok {
		return nil, fmt.Errorf("unsupported speed %d", so.Baud)
	}
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %v", path, err)
	}
	var t syscall.Termios
	if err := ioctlFile(f, syscall.TCGETS, unsafe.Pointer(&t)); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s is not a serial port: %v", path, err)
	}
	// raw mode, like cfmakeraw
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON | syscall.IXOFF | syscall.IXANY
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB | syscall.PARODD | syscall.CSTOPB | termCRTSCTS | termCBAUD
	t.Cflag |= syscall.CREAD | syscall.CLOCAL | baud
	switch so.DataBits {
	case 5:
		t.Cflag |= syscall.CS5
	case 6:
		t.Cflag |= syscall.CS6
	case 7:
		t.Cflag |= syscall.CS7
	default:
		t.Cflag |= syscall.CS8
	}
	switch so.Parity {
	case "even":
		t.Cflag |= syscall.PARENB
	case "odd":
		t.Cflag |= syscall.PARENB | syscall.PARODD
	}
	if so.StopBits == 2 {
		t.Cflag |= syscall.CSTOPB
	}
	switch so.Flow {
	case "rtscts":
		t.Cflag |= termCRTSCTS
	case "xonxoff":
		t.Iflag |= syscall.IXON | syscall.IXOFF
	}
	t.Ispeed, t.Ospeed = baud, baud
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	if err := ioctlFile(f, syscall.TCSETS, unsafe.Pointer(&t)); err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot configure %s: %v", path, err)
	}
	return f, nil
}
