package migrations

import (
	"reflect"
	"testing"
)

func TestMigrationLineEndingsPreserveChecksums(t *testing.T) {
	lf := migrationChecksums([]byte("CREATE TABLE example(id INT);\n-- stable\n"))
	crlf := migrationChecksums([]byte("CREATE TABLE example(id INT);\r\n-- stable\r\n"))
	if !reflect.DeepEqual(lf, crlf) {
		t.Fatal("Git checkout breaks existing journal")
	}
	changed := migrationChecksums([]byte("CREATE TABLE example(id TEXT);\n-- stable\n"))
	for _, a := range lf {
		for _, b := range changed {
			if a == b {
				t.Fatal("modified migration accepted")
			}
		}
	}
}
