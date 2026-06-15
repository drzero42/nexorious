package csvmap

import "testing"

// malformedQuoted is a fully quote-wrapped 3-field CSV whose third data row has
// bare inner quotes, triggering the dequoteSplit fallback.
const malformedQuoted = `"Name","Edition","Note"
"A Hat in Time","","ok"
"Episode 1: "Done Running"","","raw quotes"
`

func TestReadRecords_WellFormed_StrictPath(t *testing.T) {
	raw := []byte("Name,Status\nHalf-Life,Beaten\nPortal,Playing\n")
	recs, err := ReadRecords(raw)
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d: %v", len(recs), recs)
	}
	if recs[0][0] != "Name" || recs[1][0] != "Half-Life" || recs[2][1] != "Playing" {
		t.Fatalf("unexpected records: %v", recs)
	}
}

func TestReadRecords_MalformedQuotes_FallbackRecovers(t *testing.T) {
	recs, err := ReadRecords([]byte(malformedQuoted))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d: %v", len(recs), recs)
	}
	for i, r := range recs {
		if len(r) != 3 {
			t.Fatalf("record %d has %d fields, want 3: %v", i, len(r), r)
		}
	}
	if got := recs[2][0]; got != `Episode 1: "Done Running"` {
		t.Fatalf("recovered title = %q, want `Episode 1: \"Done Running\"`", got)
	}
}

func TestReadRecords_Windows1252_Transcoded(t *testing.T) {
	// 0xF4 is 'ô' in Windows-1252; invalid UTF-8 on its own.
	raw := []byte{'N', 'a', 'm', 'e', '\n', 'O', 'k', 'a', 'm', 'i', 0xF4, '\n'}
	recs, err := ReadRecords(raw)
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if recs[1][0] != "Okamiô" {
		t.Fatalf("transcoded = %q, want %q", recs[1][0], "Okamiô")
	}
}

func TestReadRecords_PartiallyQuoted_ReturnsError(t *testing.T) {
	// Strict parsing fails on the bare quote, and the file is NOT uniformly
	// quote-wrapped (line 2 isn't), so the fallback must not engage.
	raw := []byte("\"Name\",\"Note\"\nUnquoted: \"x\" here,plain\n")
	if _, err := ReadRecords(raw); err == nil {
		t.Fatal("want error for partially-quoted malformed file, got nil")
	}
}

func TestReadRecords_RaggedDequote_ReturnsError(t *testing.T) {
	// Uniformly quote-wrapped but a bare quote trips strict parsing AND the
	// de-quoted field counts differ (2 vs 3), so the guard rejects the fallback.
	raw := []byte("\"a\",\"b\"x\"\n\"c\",\"d\",\"e\"\n")
	if _, err := ReadRecords(raw); err == nil {
		t.Fatal("want error for ragged de-quote, got nil")
	}
}

func TestReadRecords_MalformedQuotes_CRLF_FallbackRecovers(t *testing.T) {
	// Same malformed shape but with Windows CRLF line endings.
	raw := []byte("\"Name\",\"Note\"\r\n\"Episode: \"Oops\"\",\"ok\"\r\n")
	recs, err := ReadRecords(raw)
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(recs) != 2 || recs[1][0] != `Episode: "Oops"` {
		t.Fatalf("got %v", recs)
	}
}
