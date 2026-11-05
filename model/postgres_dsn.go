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
package model

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// IsPostgresDSN reports whether a connection string is PostgreSQL.
// Keyword form (host=... dbname=... sslmode=...) is PostgreSQL, not MySQL.
func IsPostgresDSN(dsn string) bool {
	trimmed := strings.TrimSpace(dsn)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		return true
	}
	if strings.Contains(lower, "sslmode=") {
		return true
	}
	return strings.Contains(lower, "host=") && strings.Contains(lower, "dbname=")
}

// IsolateLocalPostgresDSN rewrites a local PostgreSQL DSN to a new database
// name. Remote hosts are rejected so disposable test databases cannot be
// created on a production server.
func IsolateLocalPostgresDSN(dsn, database string) (string, error) {
	if !safePostgresIdent(database) {
		return "", fmt.Errorf("invalid postgres database name %q", database)
	}
	trimmed := strings.TrimSpace(dsn)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", err
		}
		if err := requireLocalPostgresHost(parsed.Hostname()); err != nil {
			return "", err
		}
		parsed.Path = "/" + database
		return parsed.String(), nil
	}
	if !IsPostgresDSN(trimmed) {
		return "", fmt.Errorf("not a postgres dsn")
	}
	parts := strings.Fields(trimmed)
	host := ""
	foundDB := false
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			return "", fmt.Errorf("invalid postgres keyword dsn")
		}
		if strings.EqualFold(key, "host") {
			host = value
		}
		if strings.EqualFold(key, "dbname") {
			part = key + "=" + database
			foundDB = true
		}
		out = append(out, part)
	}
	if err := requireLocalPostgresHost(host); err != nil {
		return "", err
	}
	if !foundDB {
		out = append(out, "dbname="+database)
	}
	return strings.Join(out, " "), nil
}

func requireLocalPostgresHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" || strings.HasPrefix(host, "/") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("database tests only permit loopback instances")
}

func safePostgresIdent(name string) bool {
	if name == "" || len(name) > 63 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
