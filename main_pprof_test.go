/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package main

import (
	"os"
	"strings"
	"testing"
)

func TestPprofListenAddrIsLoopback(t *testing.T) {
	if got := pprofListenAddr(); got != "127.0.0.1:8005" {
		t.Fatalf("pprof listen address = %q", got)
	}
}

func TestRootComposeDoesNotPublishExampleSecrets(t *testing.T) {
	for _, name := range []string{"docker-compose.yml", "docker-compose.dev.yml"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if strings.Contains(text, "123456") {
			t.Fatalf("%s still contains the example password", name)
		}
		if strings.Contains(text, "\"3000:3000\"") || strings.Contains(text, "- \"3000:3000\"") {
			t.Fatalf("%s publishes port 3000 on every interface", name)
		}
	}
}
