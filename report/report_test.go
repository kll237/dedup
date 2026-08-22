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

func TestHuman(t *testing.T) {
	cases := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{25728, "25.12 KB"},
		{89116, "87.03 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}
	for _, c := range cases {
		if got := human(c.b); got != c.want {
			t.Errorf("human(%d) = %q, want %q", c.b, got, c.want)
		}
	}
}
