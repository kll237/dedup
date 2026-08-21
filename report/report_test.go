package report

import (
	"bytes"
	"strings"
	"testing"

	"dedup/result"
)

func sample() Report {
	return Report{
		Exact: []result.ExactGroup{
			{Hash: "abc", Size: 100, Files: []result.FileRef{{Path: "/a", Size: 100}, {Path: "/b", Size: 100}}},
		},
		Stats: result.Stats{FilesScanned: 2, ExactGroups: 1, WastedBytes: 100},
	}
}

func TestRenderText(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "精确重复") {
		t.Fatal("missing header")
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\"Exact\"") {
		t.Fatal("missing Exact field")
	}
}

func TestRenderHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, sample(), FormatHTML); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<html") {
		t.Fatal("missing html tag")
	}
}
