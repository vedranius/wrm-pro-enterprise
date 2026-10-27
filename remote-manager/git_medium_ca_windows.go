//go:build windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/x509"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mediumHostRoots reads the trusted root certificates of the Windows certificate stores
// (current user, local machine and the roots set by group policy or the enterprise),
// where proxies and antivirus software install the root they inspect HTTPS with.
func mediumHostRoots() ([]*x509.Certificate, error) {
	name, _ := windows.UTF16PtrFromString("ROOT")
	var all []*x509.Certificate
	for _, loc := range []uint32{windows.CERT_SYSTEM_STORE_CURRENT_USER, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE,
		windows.CERT_SYSTEM_STORE_CURRENT_USER_GROUP_POLICY, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE_GROUP_POLICY, windows.CERT_SYSTEM_STORE_LOCAL_MACHINE_ENTERPRISE} {
		store, e := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0, loc|windows.CERT_STORE_OPEN_EXISTING_FLAG|windows.CERT_STORE_READONLY_FLAG, uintptr(unsafe.Pointer(name)))
		if e != nil {
			continue
		}
		var ctx *windows.CertContext
		for {
			ctx, e = windows.CertEnumCertificatesInStore(store, ctx)
			if e != nil || ctx == nil {
				break
			}
			raw := unsafe.Slice(ctx.EncodedCert, ctx.Length)
			if c, err := x509.ParseCertificate(append([]byte(nil), raw...)); err == nil {
				all = append(all, c)
			}
		}
		windows.CertCloseStore(store, 0)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no certificate in the Windows root stores")
	}
	return all, nil
}
