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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeywordPostgresDSNIsNotMySQL(t *testing.T) {
	require.True(t, IsPostgresDSN("host=127.0.0.1 port=5432 user=postgres dbname=adty_test sslmode=disable"))
	require.True(t, IsPostgresDSN("postgres://postgres@127.0.0.1:5432/adty_test?sslmode=disable"))
	require.False(t, IsPostgresDSN("root:123456@tcp(127.0.0.1:3306)/new-api?parseTime=true"))
}

func TestIsolateLocalPostgresDSNRewritesKeywordAndRejectsRemote(t *testing.T) {
	rewritten, err := IsolateLocalPostgresDSN("host=/tmp/pg port=5432 user=adty dbname=postgres sslmode=disable", "newapi_audit_1")
	require.NoError(t, err)
	require.Contains(t, rewritten, "dbname=newapi_audit_1")
	require.NotContains(t, rewritten, "dbname=postgres")
	require.True(t, strings.Contains(rewritten, "host=/tmp/pg"))

	_, err = IsolateLocalPostgresDSN("host=203.0.113.10 port=5432 user=adty dbname=postgres sslmode=disable", "newapi_audit_1")
	require.Error(t, err)

	urlDSN, err := IsolateLocalPostgresDSN("postgresql://adty@127.0.0.1:5432/postgres?sslmode=disable", "newapi_audit_1")
	require.NoError(t, err)
	require.Contains(t, urlDSN, "/newapi_audit_1")
}
