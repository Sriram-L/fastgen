package runner

import (
	"bytes"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Match compares two outputs. By default it compares whitespace-separated
// tokens, which ignores trailing spaces, CRLF differences, and extra blank
// lines. exact forces a byte-for-byte comparison.
func Match(expected, got []byte, exact bool) bool {
	if exact {
		return bytes.Equal(expected, got)
	}
	return bytes.Equal(tokens(expected), tokens(got))
}

func tokens(b []byte) []byte {
	fields := bytes.Fields(b)
	return bytes.Join(fields, []byte{'\n'})
}

// DiffSummary renders a compact, line-oriented diff for the details pane.
func DiffSummary(expected, got string, maxLines int) string {
	dmp := diffmatchpatch.New()
	chars1, chars2, lineArray := dmp.DiffLinesToChars(expected, got)
	diffs := dmp.DiffMain(chars1, chars2, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var out []string
	for _, d := range diffs {
		text := strings.TrimRight(d.Text, "\n")
		if text == "" {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			switch d.Type {
			case diffmatchpatch.DiffDelete:
				out = append(out, "- "+line)
			case diffmatchpatch.DiffInsert:
				out = append(out, "+ "+line)
			default:
				out = append(out, "  "+line)
			}
		}
	}

	if maxLines > 0 && len(out) > maxLines {
		out = append(out[:maxLines], "... diff truncated ...")
	}
	if len(out) == 0 {
		return "(no differences)"
	}
	return strings.Join(out, "\n")
}
