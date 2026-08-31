package main

import (
	"testing"
	"time"
)

func TestParseRFC3339_datetimeLocalAsUTC(t *testing.T) {
	// Browser sends ISO UTC after normalization; server must parse it.
	utc, err := parseRFC3339("2026-08-31T03:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	if utc.Hour() != 3 {
		t.Fatalf("hour=%d", utc.Hour())
	}
}

func TestParseRFC3339_withOffset(t *testing.T) {
	tm, err := parseRFC3339("2026-08-31T10:00:00+07:00")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 31, 3, 0, 0, 0, time.UTC)
	if !tm.Equal(want) {
		t.Fatalf("got %v want %v", tm, want)
	}
}
