// Copyright 2026 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"net/http"
	"testing"
)

func requestWith(xff string, remoteAddr string) *http.Request {
	req := &http.Request{Header: http.Header{}, RemoteAddr: remoteAddr}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

// The proxy appends the real peer to whatever the caller sent, so the value a
// caller controls sits at the FRONT. Trusting it would let anyone past an IP
// whitelist just by sending a header.
func TestClientIpForIpWhitelistIgnoresSpoofedPrefix(t *testing.T) {
	cases := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"single hop", "203.0.113.7", "127.0.0.1:54321", "203.0.113.7"},
		{"spoofed prefix", "192.168.2.99, 203.0.113.7", "127.0.0.1:54321", "203.0.113.7"},
		{"several spoofed entries", "10.0.0.1, 10.0.0.2, 203.0.113.7", "127.0.0.1:1", "203.0.113.7"},
		{"spacing", "10.0.0.1,203.0.113.7  ", "127.0.0.1:1", "203.0.113.7"},
		{"with port", "10.0.0.1, 203.0.113.7:443", "127.0.0.1:1", "203.0.113.7"},
		{"no header falls back to peer", "", "198.51.100.9:12345", "198.51.100.9"},
		{"ipv6 peer", "", "[2001:db8::1]:443", "2001:db8::1"},
		{"ipv6 in header", "10.0.0.1, [2001:db8::2]:443", "127.0.0.1:1", "2001:db8::2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clientIpForIpWhitelist(requestWith(c.xff, c.remoteAddr)); got != c.want {
				t.Errorf("clientIpForIpWhitelist(%q, %q) = %q, want %q", c.xff, c.remoteAddr, got, c.want)
			}
		})
	}
}

// A caller must not be able to pick its own apparent address.
func TestClientIpForIpWhitelistIsNotCallerControlled(t *testing.T) {
	const realPeer = "203.0.113.7"

	for _, spoof := range []string{"192.168.2.12", "127.0.0.1", "10.0.0.0", "not-an-ip"} {
		got := clientIpForIpWhitelist(requestWith(spoof+", "+realPeer, "127.0.0.1:1"))
		if got == spoof {
			t.Errorf("a caller sending %q was able to control the resolved IP", spoof)
		}
		if got != realPeer {
			t.Errorf("expected the proxy-appended peer %q, got %q", realPeer, got)
		}
	}
}
