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

import "testing"

func TestAdtyDSNAcceptsKeywordPostgreSQL(t *testing.T) {
	keyword := "host=127.0.0.1 port=5432 user=postgres dbname=adty sslmode=disable"
	if !adtyDSNIsPostgreSQL(keyword) {
		t.Fatal("keyword PostgreSQL DSN was rejected")
	}
	if !adtyDSNIsPostgreSQL("postgresql://adty@127.0.0.1:5432/adty?sslmode=disable") {
		t.Fatal("URL PostgreSQL DSN was rejected")
	}
	if adtyDSNIsPostgreSQL("root:pass@tcp(127.0.0.1:3306)/new-api") {
		t.Fatal("MySQL DSN was accepted as PostgreSQL")
	}
}
