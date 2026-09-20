package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"flashcard_lambda/internal/migration"
)

func TestMigrationFlagsDefaultToDryRunAndRequireExplicitApplySources(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{"dry run", []string{"--table", "flashcards", "--bucket", "managed-bucket"}, false},
		{"missing table", []string{"--bucket", "managed-bucket"}, true},
		{"missing bucket", []string{"--table", "flashcards"}, true},
		{"unsafe apply", []string{"--table", "flashcards", "--bucket", "managed-bucket", "--apply"}, true},
		{"explicit apply", []string{"--table", "flashcards", "--bucket", "managed-bucket", "--apply", "--source-buckets", "legacy-bucket"}, false},
		{"empty list entry", []string{"--table", "flashcards", "--bucket", "managed-bucket", "--apply", "--source-buckets", "legacy-bucket,"}, true},
		{"excessive scan", []string{"--table", "flashcards", "--bucket", "managed-bucket", "--max-pages", "10001"}, true},
		{"extra argument", []string{"--table", "flashcards", "--bucket", "managed-bucket", "unexpected"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options, err := parseOptions(tc.args, &bytes.Buffer{})
			if (err != nil) != tc.wantError {
				t.Fatalf("options=%+v err=%v", options, err)
			}
			if tc.name == "dry run" && options.config.Apply {
				t.Fatal("default mode writes changes")
			}
		})
	}
}

func TestReportFileIsPrivateAndNeverOverwritesExistingEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := migration.Report{Mode: "dry-run", Planned: 1, Entries: []migration.Entry{{ID: "8c47d801-dd96-449c-8b47-7fe3f7fb917e", Action: "planned"}}}
	if err := writeReport(path, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got migration.Report
	if err := json.Unmarshal(data, &got); err != nil || got.Planned != 1 {
		t.Fatalf("invalid report: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("report permissions=%o", info.Mode().Perm())
	}
	if err := writeReport(path, migration.Report{}); err == nil {
		t.Fatal("overwrote existing report")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("original report was not preserved")
	}
}
