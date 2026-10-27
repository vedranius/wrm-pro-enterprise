// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── TERMINAL BACKENDS ───────────────────────────────
//
// The terminal WebSocket (ssh_ws.go) talks to one of these:
//   - an SSH shell (the normal case);
//   - the serial console of the server through its BMC's SSH: WRM logs in to the BMC and
//     types the console command (iDRAC "console com2", iLO "vsp", …);
//   - IPMI Serial-over-LAN: "ipmitool sol activate", on the last jump host (through SSH) or
//     on the WRM server (Linux, in a pseudo terminal);
//   - a serial port of the WRM server (connections of protocol SERIAL, Linux): console
//     cables to switches, routers and appliances.

type termIO struct {
	stdin   io.Writer
	stdout  io.Reader
	stderr  io.Reader                  // optional
	start   func() error               // starts the shell / command
	resize  func(cols, rows int)       // window size changed
	wait    func() (code int, ok bool) // after stdout ended: exit code, or ok=false when the link was lost
	alive   func() bool                // keepalive (nil: not needed)
	close   func()                     // ends everything (also used to kill)
	typeCmd string                     // typed once the output is quiet (BMC console command)
	banner  string                     // printed before connecting
}

var termModes = ssh.TerminalModes{
	ssh.ECHO: 1, ssh.ICANON: 1, ssh.ISIG: 1, ssh.IEXTEN: 1,
	ssh.ICRNL: 1, ssh.IMAXBEL: 1, ssh.IXON: 1, ssh.IXANY: 1, ssh.IUTF8: 1,
	ssh.OPOST: 1, ssh.ONLCR: 1, ssh.OCRNL: 0, ssh.ONLRET: 0,
	ssh.CS8: 1, ssh.PARENB: 0,
	ssh.VINTR: 3, ssh.VQUIT: 28, ssh.VERASE: 127, ssh.VKILL: 21, ssh.VEOF: 4,
	ssh.VSTART: 17, ssh.VSTOP: 19, ssh.VSUSP: 26, ssh.VREPRINT: 18,
	ssh.VWERASE: 23, ssh.VLNEXT: 22, ssh.VDISCARD: 15,
	ssh.TTY_OP_ISPEED: 115200, ssh.TTY_OP_OSPEED: 115200,
}

// sshTerm opens a PTY session on an SSH client; command "" starts the login shell.
func sshTerm(cl *ssh.Client, cols, rows int, term, command string) (*termIO, error) {
	session, err := cl.NewSession()
	if err != nil {
		cl.Close()
		return nil, fmt.Errorf("session error: %v", err)
	}
	if err := session.RequestPty(term, rows, cols, termModes); err != nil {
		cl.Close()
		return nil, fmt.Errorf("PTY error: %v", err)
	}
	session.Setenv("LANG", "C.UTF-8") // best effort, usually refused by AcceptEnv
	in, _ := session.StdinPipe()
	out, _ := session.StdoutPipe()
	errp, _ := session.StderrPipe()
	var closeOnce sync.Once
	return &termIO{stdin: in, stdout: out, stderr: errp,
		start: func() error {
			if command != "" {
				return session.Start(command)
			}
			return session.Shell()
		},
		resize: func(c, r int) { session.WindowChange(r, c) },
		wait: func() (int, bool) {
			waitErr := make(chan error, 1)
			go func() { waitErr <- session.Wait() }()
			select {
			case err := <-waitErr:
				var ee *ssh.ExitError
				if err == nil {
					return 0, true
				} else if errors.As(err, &ee) {
					return ee.ExitStatus(), true
				}
				return -1, false
			case <-time.After(2 * time.Second):
				return -1, false
			}
		},
		alive: func() bool { return sshAlive(cl, 15*time.Second) },
		close: func() { closeOnce.Do(func() { session.Close(); cl.Close() }) },
	}, nil
}

// bmcSSHConn is the BMC itself as an SSH connection (port 22 of the BMC address).
func bmcSSHConn(c Connection, b bmcConfig) (Connection, error) {
	user, pass, err := bmcLogin(c, b)
	if err != nil {
		return Connection{}, err
	}
	host, port := hostOnly(b.Host), "22"
	if b.SSHPort > 0 {
		port = strconv.Itoa(b.SSHPort)
	}
	bc := Connection{ID: 0, Name: c.Name + " (BMC)", Protocol: "SSH", Host: joinHostPortSafe(host, port), Username: user, Password: pass,
		AuthMethod: "PASSWORD", UserID: c.UserID, JumpID: bmcJump(c, b), ProxyID: bmcProxy(c, b)}
	return bc, nil
}

func joinHostPortSafe(host, port string) string {
	if strings.Contains(host, ":") {
		return "[" + strings.Trim(host, "[]") + "]:" + port
	}
	return host + ":" + port
}

// solMarker is printed by the SOL wrapper on the jump host once echo is off.
const solMarker = "WRM-SOL-READY"

// markerReader discards output until the marker, then sends the secret and passes the
// rest through (the password never shows up in the terminal or the recording).
type markerReader struct {
	r      io.Reader
	w      io.Writer
	secret string
	done   bool
	buf    []byte
}

func (m *markerReader) Read(p []byte) (int, error) {
	if m.done {
		if len(m.buf) > 0 {
			n := copy(p, m.buf)
			m.buf = m.buf[n:]
			return n, nil
		}
		return m.r.Read(p)
	}
	tmp := make([]byte, 4096)
	deadline := time.Now().Add(20 * time.Second)
	for !m.done {
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("the console did not start")
		}
		n, err := m.r.Read(tmp)
		m.buf = append(m.buf, tmp[:n]...)
		if i := strings.Index(string(m.buf), solMarker); i >= 0 {
			rest := m.buf[i+len(solMarker):]
			rest = []byte(strings.TrimLeft(string(rest), "\r\n"))
			m.buf = append([]byte{}, rest...)
			m.done = true
			io.WriteString(m.w, m.secret+"\n")
			break
		}
		if err != nil {
			// e.g. "ipmitool is not installed": show what the jump host said
			m.done = true
			if len(m.buf) == 0 {
				return 0, err
			}
			break
		}
		if len(m.buf) > 64<<10 {
			m.buf = m.buf[len(m.buf)-64:]
		}
	}
	return m.Read(p)
}

// openConsole opens the serial console of connection c ("bmc" or "sol").
func openConsole(c Connection, kind string, cols, rows int, term string, onNewKey func(host, fp string)) (*termIO, error) {
	b, ok := loadBMC(c.ID)
	if !ok {
		return nil, fmt.Errorf("no BMC is set for this connection (Edit → Out-of-band management)")
	}
	switch kind {
	case "bmc":
		bc, err := bmcSSHConn(c, b)
		if err != nil {
			return nil, err
		}
		cl, err := dialSSH(bc, onNewKey)
		if err != nil {
			return nil, err
		}
		tio, err := sshTerm(cl, cols, rows, term, "")
		if err != nil {
			return nil, err
		}
		tio.typeCmd = b.Console
		if tio.typeCmd == "" {
			tio.banner = "Logged in to the BMC. Start the console with its command (iDRAC: console com2, iLO: vsp, XClarity: console 1) or set it in the connection."
		}
		return tio, nil
	case "sol":
		if b.Type != "ipmi" {
			return nil, fmt.Errorf("Serial-over-LAN needs an IPMI BMC; for Redfish BMCs use the console over the BMC's SSH")
		}
		user, pass, err := bmcLogin(c, b)
		if err != nil {
			return nil, err
		}
		args := append(ipmiArgs(b, user), "sol", "activate")
		if err := ipmiProxyRefused(c, b); err != nil {
			return nil, err
		}
		hop, err := bmcHop(c, b)
		if err != nil {
			return nil, err
		}
		if hop != nil {
			q := make([]string, len(args))
			for i, a := range args {
				q[i] = shellQuote(a)
			}
			script := `stty -echo 2>/dev/null; echo ` + solMarker + `; IFS= read -r IPMI_PASSWORD; stty echo 2>/dev/null; export IPMI_PASSWORD
command -v ipmitool >/dev/null 2>&1 || { echo "ipmitool is not installed on the jump host"; exit 127; }
ipmitool ` + strings.Join(q[:len(q)-2], " ") + ` sol deactivate >/dev/null 2>&1
exec ipmitool ` + strings.Join(q, " ")
			tio, err := sshTerm(hop, cols, rows, term, "sh -c "+shellQuote(script))
			if err != nil {
				return nil, err
			}
			tio.stdout = &markerReader{r: tio.stdout, w: tio.stdin, secret: pass}
			tio.banner = "Serial-over-LAN through the jump host. ~. ends the console."
			return tio, nil
		}
		path, lerr := exec.LookPath(ipmitoolPath)
		if lerr != nil {
			return nil, fmt.Errorf("ipmitool is not installed on the WRM server (apt install ipmitool), or reach the BMC through a jump host that has it")
		}
		deact := exec.Command(path, append(ipmiArgs(b, user), "sol", "deactivate")...)
		deact.Env = append(os.Environ(), "IPMI_PASSWORD="+pass)
		deact.Run()
		cmd := exec.Command(path, args...)
		cmd.Env = append(os.Environ(), "IPMI_PASSWORD="+pass, "TERM="+term)
		tio, err := ptyTerm(cmd, cols, rows)
		if err != nil {
			return nil, err
		}
		tio.banner = "Serial-over-LAN. ~. ends the console."
		return tio, nil
	}
	return nil, fmt.Errorf("unknown console %q", kind)
}

// ptyTerm runs a local command in a pseudo terminal.
func ptyTerm(cmd *exec.Cmd, cols, rows int) (*termIO, error) {
	master, err := startInPTY(cmd, cols, rows)
	if err != nil {
		return nil, err
	}
	var closeOnce sync.Once
	exited := make(chan struct{})
	var exitCode int
	go func() {
		err := cmd.Wait()
		exitCode = 0
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else if err != nil {
			exitCode = -1
		}
		close(exited)
	}()
	return &termIO{stdin: master, stdout: master,
		start:  func() error { return nil },
		resize: func(c, r int) { setPTYSize(master, c, r) },
		wait: func() (int, bool) {
			select {
			case <-exited:
				return exitCode, true
			case <-time.After(2 * time.Second):
				return -1, true
			}
		},
		close: func() {
			closeOnce.Do(func() {
				if cmd.Process != nil {
					cmd.Process.Kill()
				}
				master.Close()
			})
		},
	}, nil
}

// ── serial ports (protocol SERIAL) ──

var serialNameRe = regexp.MustCompile(`^tty[A-Za-z]{0,10}[0-9]{1,3}$`)

// serialDevDir is where serial devices live (a test points it elsewhere).
var serialDevDir = "/dev/"

type serialOptions struct {
	Baud     int
	DataBits int
	Parity   string // none | even | odd
	StopBits int
	Flow     string // none | rtscts | xonxoff
}

func serialOptionsFrom(o map[string]string) serialOptions {
	so := serialOptions{Baud: 9600, DataBits: 8, Parity: "none", StopBits: 1, Flow: "none"}
	if v, err := strconv.Atoi(o["baud"]); err == nil {
		so.Baud = v
	}
	if v, err := strconv.Atoi(o["data_bits"]); err == nil {
		so.DataBits = v
	}
	if v := o["parity"]; v != "" {
		so.Parity = v
	}
	if v, err := strconv.Atoi(o["stop_bits"]); err == nil {
		so.StopBits = v
	}
	if v := o["flow"]; v != "" {
		so.Flow = v
	}
	return so
}

// serialAllowed: serial ports belong to the WRM server, like key files.
func serialAllowed(userID int) bool {
	return policyWhoAllows(userID, "serial_ports")
}

func openSerial(c Connection) (*termIO, error) {
	name := hostOnly(c.Host)
	if !serialNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid serial port %q (e.g. ttyUSB0, ttyS0, ttyACM0)", name)
	}
	return openSerialPath(serialDevDir+name, serialOptionsFrom(loadDesktopOptions(c.ID)))
}

func openSerialPath(path string, so serialOptions) (*termIO, error) {
	f, err := openSerialPort(path, so)
	if err != nil {
		return nil, err
	}
	var closeOnce sync.Once
	return &termIO{stdin: f, stdout: f,
		start:  func() error { return nil },
		resize: func(int, int) {},
		wait:   func() (int, bool) { return 0, false },
		close:  func() { closeOnce.Do(func() { f.Close() }) },
		banner: fmt.Sprintf("Serial port %s, %d %d%s%d, flow %s.", path, so.Baud, so.DataBits, strings.ToUpper(so.Parity[:1]), so.StopBits, so.Flow),
	}, nil
}

var serialSpeeds = []int{1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600}

// normalizeSerialOptions validates the line settings of a SERIAL connection.
func normalizeSerialOptions(in map[string]string) (map[string]string, error) {
	so := serialOptionsFrom(in)
	ok := false
	for _, s := range serialSpeeds {
		ok = ok || s == so.Baud
	}
	if !ok {
		return nil, fmt.Errorf("unsupported speed %d", so.Baud)
	}
	if so.DataBits < 5 || so.DataBits > 8 {
		return nil, fmt.Errorf("data bits must be 5 to 8")
	}
	if so.Parity != "none" && so.Parity != "even" && so.Parity != "odd" {
		return nil, fmt.Errorf("parity must be none, even or odd")
	}
	if so.StopBits != 1 && so.StopBits != 2 {
		return nil, fmt.Errorf("stop bits must be 1 or 2")
	}
	if so.Flow != "none" && so.Flow != "rtscts" && so.Flow != "xonxoff" {
		return nil, fmt.Errorf("flow control must be none, rtscts or xonxoff")
	}
	return map[string]string{"baud": strconv.Itoa(so.Baud), "data_bits": strconv.Itoa(so.DataBits), "parity": so.Parity,
		"stop_bits": strconv.Itoa(so.StopBits), "flow": so.Flow}, nil
}
