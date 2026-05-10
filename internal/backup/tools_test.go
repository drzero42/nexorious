package backup

import "testing"

func TestCheckTools_SetsAvailability(t *testing.T) {
	CheckTools()
	_ = PgDumpAvailable()
	_ = PsqlAvailable()
}

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want DBConnParams
	}{
		{
			name: "standard postgres URL",
			url:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
		{
			name: "postgresql scheme",
			url:  "postgresql://admin:secret@db.example.com:5433/proddb",
			want: DBConnParams{Host: "db.example.com", Port: "5433", User: "admin", Password: "secret", DBName: "proddb"},
		},
		{
			name: "default port",
			url:  "postgres://user:pass@localhost/mydb",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDatabaseURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Host != tt.want.Host || got.Port != tt.want.Port || got.User != tt.want.User || got.Password != tt.want.Password || got.DBName != tt.want.DBName {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
