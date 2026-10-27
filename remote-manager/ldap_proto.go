// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// Minimal LDAPv3 client (RFC 4511) for directory sign-in: simple bind, search with paged
// results, StartTLS. Only the parts WRM needs are implemented; the encoding is BER with
// definite lengths, which is what every LDAP server sends.

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ─── BER ─────────────────────────────────────────────

const (
	berUniversal   = 0x00
	berApplication = 0x40
	berContext     = 0x80
	berConstructed = 0x20

	berTagBoolean     = 0x01
	berTagInteger     = 0x02
	berTagOctetString = 0x04
	berTagEnumerated  = 0x0a
	berTagSequence    = 0x10 | berConstructed
	berTagSet         = 0x11 | berConstructed

	berMaxLen = 16 << 20
)

type berPacket struct {
	Tag      byte // class | constructed | number (numbers < 31 only)
	Value    []byte
	Children []*berPacket
}

func berNew(tag byte, children ...*berPacket) *berPacket {
	return &berPacket{Tag: tag | berConstructed, Children: children}
}
func berOctets(tag byte, v []byte) *berPacket { return &berPacket{Tag: tag, Value: v} }
func berString(tag byte, s string) *berPacket { return &berPacket{Tag: tag, Value: []byte(s)} }
func berStr(s string) *berPacket              { return berString(berTagOctetString, s) }
func berBool(tag byte, b bool) *berPacket {
	if b {
		return &berPacket{Tag: tag, Value: []byte{0xff}}
	}
	return &berPacket{Tag: tag, Value: []byte{0}}
}
func berInt(tag byte, n int64) *berPacket {
	// Two's complement, shortest form.
	if n < 0 {
		out := berMagnitude(^n) // -n-1 >= 0
		for i := range out {
			out[i] = ^out[i]
		}
		if out[0]&0x80 == 0 {
			out = append([]byte{0xff}, out...)
		}
		return &berPacket{Tag: tag, Value: out}
	}
	out := berMagnitude(n)
	if out[0]&0x80 != 0 {
		out = append([]byte{0}, out...)
	}
	return &berPacket{Tag: tag, Value: out}
}

// berMagnitude returns the big-endian bytes of n >= 0, at least one byte.
func berMagnitude(n int64) []byte {
	b := new(big.Int).SetInt64(n).Bytes()
	if len(b) == 0 {
		return []byte{0}
	}
	return b
}

// berLongForm[k] is the first length octet of a long-form length with k+1 length bytes.
var berLongForm = [...]byte{0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88}

func (p *berPacket) constructed() bool { return p.Tag&berConstructed != 0 }

func (p *berPacket) Bytes() []byte {
	body := p.Value
	if p.constructed() {
		var b bytes.Buffer
		for _, c := range p.Children {
			b.Write(c.Bytes())
		}
		body = b.Bytes()
	}
	out := []byte{p.Tag}
	lb := berMagnitude(int64(len(body)))
	if len(body) >= 0x80 {
		out = append(out, berLongForm[len(lb)-1])
	}
	out = append(out, lb...)
	return append(out, body...)
}

func (p *berPacket) Int() int64 {
	var n int64
	for i, b := range p.Value {
		if i == 0 && b&0x80 != 0 {
			n = -1
		}
		n = n<<8 | int64(b)
	}
	return n
}

func (p *berPacket) Str() string { return string(p.Value) }

func (p *berPacket) child(i int) *berPacket {
	if i < len(p.Children) {
		return p.Children[i]
	}
	return &berPacket{}
}

func berRead(r io.Reader) (*berPacket, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if hdr[0]&0x1f == 0x1f {
		return nil, errors.New("ldap: multi-byte BER tags are not supported")
	}
	n := int(hdr[1])
	if n&0x80 != 0 {
		k := n & 0x7f
		if k == 0 || k > 4 {
			return nil, errors.New("ldap: unsupported BER length")
		}
		var lb [4]byte
		if _, err := io.ReadFull(r, lb[:k]); err != nil {
			return nil, err
		}
		n = 0
		for _, b := range lb[:k] {
			n = n<<8 | int(b)
		}
	}
	if n < 0 || n > berMaxLen {
		return nil, errors.New("ldap: message too large")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return berParse(hdr[0], body)
}

func berParse(tag byte, body []byte) (*berPacket, error) {
	p := &berPacket{Tag: tag, Value: body}
	if tag&berConstructed == 0 {
		return p, nil
	}
	rd := bytes.NewReader(body)
	for rd.Len() > 0 {
		c, err := berRead(rd)
		if err != nil {
			return nil, err
		}
		p.Children = append(p.Children, c)
	}
	p.Value = nil
	return p, nil
}

// ─── filters (RFC 4515) ──────────────────────────────

// ldapEscape escapes a value for use inside a search filter.
func ldapEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', '*', '(', ')', 0:
			fmt.Fprintf(&b, "\\%02x", c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ldapEscapeBytes escapes every byte (for binary values such as objectGUID).
func ldapEscapeBytes(v []byte) string {
	var b strings.Builder
	for _, c := range v {
		fmt.Fprintf(&b, "\\%02x", c)
	}
	return b.String()
}

func ldapUnescape(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out = append(out, s[i])
			continue
		}
		if i+2 >= len(s) {
			return nil, errors.New("bad escape in filter")
		}
		v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return nil, errors.New("bad escape in filter")
		}
		out = append(out, byte(v))
		i += 2
	}
	return out, nil
}

func ldapCompileFilter(f string) (*berPacket, error) {
	f = strings.TrimSpace(f)
	if !strings.HasPrefix(f, "(") {
		f = "(" + f + ")"
	}
	p, rest, err := ldapFilterAt(f, 0)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rest) != "" {
		return nil, errors.New("unexpected text after the filter")
	}
	return p, nil
}

func ldapFilterAt(f string, depth int) (*berPacket, string, error) {
	if depth > 30 {
		return nil, "", errors.New("filter nested too deeply")
	}
	if !strings.HasPrefix(f, "(") {
		return nil, "", errors.New("filter must start with (")
	}
	f = f[1:]
	if f == "" {
		return nil, "", errors.New("unterminated filter")
	}
	switch f[0] {
	case '&', '|':
		tag := byte(berContext)
		if f[0] == '|' {
			tag = berContext | 1
		}
		p := berNew(tag)
		f = f[1:]
		for strings.HasPrefix(f, "(") {
			c, rest, err := ldapFilterAt(f, depth+1)
			if err != nil {
				return nil, "", err
			}
			p.Children = append(p.Children, c)
			f = rest
		}
		if !strings.HasPrefix(f, ")") {
			return nil, "", errors.New("missing ) in filter")
		}
		return p, f[1:], nil
	case '!':
		c, rest, err := ldapFilterAt(f[1:], depth+1)
		if err != nil {
			return nil, "", err
		}
		if !strings.HasPrefix(rest, ")") {
			return nil, "", errors.New("missing ) in filter")
		}
		return berNew(berContext|2, c), rest[1:], nil
	}
	end := strings.IndexByte(f, ')')
	if end < 0 {
		return nil, "", errors.New("missing ) in filter")
	}
	item, rest := f[:end], f[end+1:]
	eq := strings.IndexByte(item, '=')
	if eq <= 0 {
		return nil, "", fmt.Errorf("bad filter item %q", item)
	}
	attr, val := item[:eq], item[eq+1:]
	op := byte('=')
	if l := attr[len(attr)-1]; l == '>' || l == '<' || l == '~' || l == ':' {
		op, attr = l, attr[:len(attr)-1]
	}
	for _, ch := range attr {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == ';' || ch == '.' || ch == ':' && op == ':') {
			return nil, "", fmt.Errorf("bad attribute in filter item %q", item)
		}
	}
	if attr == "" {
		return nil, "", fmt.Errorf("bad filter item %q", item)
	}
	switch op {
	case ':':
		// attr:rule:=value (extensible match; WRM uses it for AD's in-chain rule)
		parts := strings.SplitN(attr, ":", 2)
		v, err := ldapUnescape(val)
		if err != nil {
			return nil, "", err
		}
		p := berNew(berContext | 9)
		if len(parts) == 2 && parts[1] != "" {
			p.Children = append(p.Children, berString(berContext|1, parts[1]))
		}
		p.Children = append(p.Children, berString(berContext|2, parts[0]), berOctets(berContext|3, v))
		return p, rest, nil
	case '>', '<', '~':
		v, err := ldapUnescape(val)
		if err != nil {
			return nil, "", err
		}
		tag := map[byte]byte{'>': 5, '<': 6, '~': 8}[op]
		return berNew(berContext|tag, berStr(attr), berOctets(berTagOctetString, v)), rest, nil
	}
	if val == "*" {
		return berString(berContext|7, attr), rest, nil
	}
	if strings.Contains(val, "*") {
		parts := strings.Split(val, "*")
		subs := berNew(berTagSequence)
		for i, s := range parts {
			if s == "" {
				continue
			}
			v, err := ldapUnescape(s)
			if err != nil {
				return nil, "", err
			}
			tag := byte(berContext | 1)
			if i == 0 {
				tag = berContext
			} else if i == len(parts)-1 {
				tag = berContext | 2
			}
			subs.Children = append(subs.Children, berOctets(tag, v))
		}
		return berNew(berContext|4, berStr(attr), subs), rest, nil
	}
	v, err := ldapUnescape(val)
	if err != nil {
		return nil, "", err
	}
	return berNew(berContext|3, berStr(attr), berOctets(berTagOctetString, v)), rest, nil
}

// ─── client ──────────────────────────────────────────

const (
	ldapOpBindRequest    = berApplication
	ldapOpBindResponse   = berApplication | 1
	ldapOpUnbind         = berApplication | 2
	ldapOpSearchRequest  = berApplication | 3
	ldapOpSearchEntry    = berApplication | 4
	ldapOpSearchDone     = berApplication | 5
	ldapOpSearchRef      = berApplication | 19
	ldapOpExtendedReq    = berApplication | 23
	ldapOpExtendedResp   = berApplication | 24
	ldapOIDStartTLS      = "1.3.6.1.4.1.1466.20037"
	ldapOIDPagedResults  = "1.2.840.113556.1.4.319"
	ldapResultInvalidCrd = 49
)

type ldapError struct {
	Code    int
	Message string
}

func (e *ldapError) Error() string {
	name := map[int]string{1: "operations error", 2: "protocol error", 3: "time limit", 4: "size limit", 8: "stronger authentication required",
		32: "no such object", 34: "invalid DN syntax", 48: "inappropriate authentication", 49: "invalid credentials", 50: "insufficient access rights",
		51: "busy", 52: "unavailable", 53: "unwilling to perform"}[e.Code]
	if name == "" {
		name = "result " + strconv.Itoa(e.Code)
	}
	if e.Message != "" {
		return "LDAP " + name + ": " + e.Message
	}
	return "LDAP " + name
}

type ldapEntry struct {
	DN    string
	Attrs map[string][][]byte // lower-case attribute name → values
}

func (e *ldapEntry) get(attr string) string {
	if v := e.Attrs[strings.ToLower(attr)]; len(v) > 0 {
		return string(v[0])
	}
	return ""
}

func (e *ldapEntry) all(attr string) []string {
	var out []string
	for _, v := range e.Attrs[strings.ToLower(attr)] {
		out = append(out, string(v))
	}
	return out
}

func (e *ldapEntry) raw(attr string) []byte {
	if v := e.Attrs[strings.ToLower(attr)]; len(v) > 0 {
		return v[0]
	}
	return nil
}

type ldapConn struct {
	conn    net.Conn
	rd      *bufio.Reader
	msgID   int64
	timeout time.Duration
	TLS     *tls.ConnectionState
}

type ldapDialOptions struct {
	URL      string // ldaps://host[:636] or ldap://host[:389] (StartTLS is then mandatory)
	CAPEM    string // extra trusted CA certificates (optional)
	Timeout  time.Duration
	DialFunc func(network, addr string) (net.Conn, error) // tests
}

func ldapParseURL(raw string) (host, port string, implicitTLS bool, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", "", false, errors.New("enter the server as ldaps://host or ldap://host")
	}
	switch strings.ToLower(u.Scheme) {
	case "ldaps":
		implicitTLS, port = true, "636"
	case "ldap":
		port = "389"
	default:
		return "", "", false, errors.New("the address must start with ldaps:// or ldap://")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return "", "", false, errors.New("enter only the scheme, host and port (put the base DN in its own field)")
	}
	host = u.Hostname()
	if p := u.Port(); p != "" {
		port = p
	}
	return host, port, implicitTLS, nil
}

func ldapTLSConfig(host, caPEM string) (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if strings.TrimSpace(caPEM) != "" && !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, errors.New("the CA certificate is not valid PEM")
	}
	return &tls.Config{ServerName: host, RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// ldapDial connects with TLS: ldaps:// directly, ldap:// through StartTLS. There is no
// plain-text mode and certificate checks cannot be turned off.
func ldapDial(o ldapDialOptions) (*ldapConn, error) {
	host, port, implicitTLS, err := ldapParseURL(o.URL)
	if err != nil {
		return nil, err
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	cfg, err := ldapTLSConfig(host, o.CAPEM)
	if err != nil {
		return nil, err
	}
	dial := o.DialFunc
	if dial == nil {
		d := &net.Dialer{Timeout: o.Timeout}
		dial = d.Dial
	}
	raw, err := dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	c := &ldapConn{conn: raw, rd: bufio.NewReader(raw), timeout: o.Timeout}
	if !implicitTLS {
		if err := c.startTLS(); err != nil {
			raw.Close()
			return nil, fmt.Errorf("StartTLS: %w", err)
		}
	}
	tc := tls.Client(raw, cfg)
	tc.SetDeadline(time.Now().Add(o.Timeout))
	if err := tc.Handshake(); err != nil {
		raw.Close()
		return nil, fmt.Errorf("TLS: %w", err)
	}
	tc.SetDeadline(time.Time{})
	st := tc.ConnectionState()
	c.conn, c.rd, c.TLS = tc, bufio.NewReader(tc), &st
	return c, nil
}

func (c *ldapConn) Close() {
	if c == nil || c.conn == nil {
		return
	}
	c.msgID++
	c.conn.SetDeadline(time.Now().Add(2 * time.Second))
	c.conn.Write(berNew(berTagSequence, berInt(berTagInteger, c.msgID), &berPacket{Tag: ldapOpUnbind}).Bytes())
	c.conn.Close()
}

func (c *ldapConn) send(op *berPacket, controls ...*berPacket) (int64, error) {
	c.msgID++
	msg := berNew(berTagSequence, berInt(berTagInteger, c.msgID), op)
	if len(controls) > 0 {
		msg.Children = append(msg.Children, berNew(berContext, controls...))
	}
	c.conn.SetDeadline(time.Now().Add(c.timeout))
	_, err := c.conn.Write(msg.Bytes())
	return c.msgID, err
}

func (c *ldapConn) recv(id int64) (*berPacket, error) {
	for {
		c.conn.SetDeadline(time.Now().Add(c.timeout))
		p, err := berRead(c.rd)
		if err != nil {
			return nil, err
		}
		if len(p.Children) < 2 {
			return nil, errors.New("ldap: malformed message")
		}
		if p.Children[0].Int() == id {
			return p, nil
		}
		if p.Children[0].Int() == 0 { // notice of disconnection
			return nil, errors.New("ldap: the server closed the connection")
		}
	}
}

func ldapResult(op *berPacket) error {
	if code := int(op.child(0).Int()); code != 0 {
		return &ldapError{Code: code, Message: op.child(2).Str()}
	}
	return nil
}

func (c *ldapConn) startTLS() error {
	id, err := c.send(berNew(ldapOpExtendedReq, berString(berContext, ldapOIDStartTLS)))
	if err != nil {
		return err
	}
	p, err := c.recv(id)
	if err != nil {
		return err
	}
	if p.Children[1].Tag != ldapOpExtendedResp|berConstructed {
		return errors.New("unexpected answer")
	}
	return ldapResult(p.Children[1])
}

// Bind does a simple bind. An empty password is refused here: servers treat it as an
// anonymous ("unauthenticated") bind that succeeds, which must never count as a sign-in.
func (c *ldapConn) Bind(dn, password string) error {
	if dn == "" || password == "" {
		return &ldapError{Code: ldapResultInvalidCrd, Message: "empty name or password"}
	}
	id, err := c.send(berNew(ldapOpBindRequest, berInt(berTagInteger, 3), berStr(dn), berString(berContext, password)))
	if err != nil {
		return err
	}
	p, err := c.recv(id)
	if err != nil {
		return err
	}
	if p.Children[1].Tag != ldapOpBindResponse|berConstructed {
		return errors.New("ldap: unexpected bind answer")
	}
	return ldapResult(p.Children[1])
}

const (
	ldapScopeBase = 0
	ldapScopeSub  = 2
)

// Search runs a search and follows paged results up to limit entries.
func (c *ldapConn) Search(base string, scope int, filter string, attrs []string, limit int) ([]*ldapEntry, error) {
	fp, err := ldapCompileFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("filter %s: %w", filter, err)
	}
	if limit <= 0 {
		limit = 1000
	}
	var out []*ldapEntry
	var cookie []byte
	for page := 0; page < 1000; page++ {
		al := berNew(berTagSequence)
		for _, a := range attrs {
			al.Children = append(al.Children, berStr(a))
		}
		req := berNew(ldapOpSearchRequest, berStr(base), berInt(berTagEnumerated, int64(scope)), berInt(berTagEnumerated, 0),
			berInt(berTagInteger, 0), berInt(berTagInteger, 30), berBool(berTagBoolean, false), fp, al)
		pv := berNew(berTagSequence, berInt(berTagInteger, 500), berOctets(berTagOctetString, cookie))
		ctrl := berNew(berTagSequence, berStr(ldapOIDPagedResults), berOctets(berTagOctetString, pv.Bytes()))
		id, err := c.send(req, ctrl)
		if err != nil {
			return nil, err
		}
		cookie = nil
	messages:
		for {
			p, err := c.recv(id)
			if err != nil {
				return nil, err
			}
			op := p.Children[1]
			switch op.Tag {
			case ldapOpSearchEntry | berConstructed:
				e := &ldapEntry{DN: op.child(0).Str(), Attrs: map[string][][]byte{}}
				for _, a := range op.child(1).Children {
					k := strings.ToLower(a.child(0).Str())
					for _, v := range a.child(1).Children {
						e.Attrs[k] = append(e.Attrs[k], v.Value)
					}
				}
				if len(out) < limit {
					out = append(out, e)
				}
				continue
			case ldapOpSearchRef | berConstructed:
				continue // referrals are not followed
			case ldapOpSearchDone | berConstructed:
				if err := ldapResult(op); err != nil {
					if le, ok := err.(*ldapError); ok && le.Code == 4 && len(out) > 0 {
						return out, nil
					}
					return out, err
				}
				if len(p.Children) > 2 {
					for _, ctl := range p.Children[2].Children {
						if ctl.child(0).Str() != ldapOIDPagedResults {
							continue
						}
						v := ctl.Children[len(ctl.Children)-1]
						if pp, err := berRead(bytes.NewReader(v.Value)); err == nil {
							cookie = pp.child(1).Value
						}
					}
				}
				break messages
			default:
				return out, errors.New("ldap: unexpected search answer")
			}
		}
		if len(cookie) == 0 || len(out) >= limit {
			return out, nil
		}
	}
	return out, nil
}
