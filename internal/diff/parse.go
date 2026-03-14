package diff

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	diffGitPrefix = "diff --git "
	hunkPrefix    = "@@"
	addedPrefix   = "+"
	removedPrefix = "-"
	devNull       = "/dev/null"
)

func Parse(diffText string) ([]File, error) {
	lines := strings.Split(diffText, "\n")
	var files []File
	var current *File

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, diffGitPrefix) {
			path := extractPath(line)
			newFile := File{Path: path}
			files = append(files, newFile)
			current = &files[len(files)-1]
			current.Path = resolvePathFromHeaders(lines, i, path)
			continue
		}

		if current == nil {
			continue
		}

		if !strings.HasPrefix(line, hunkPrefix) {
			continue
		}

		hunk, err := parseHunkHeader(line)
		if err != nil {
			return nil, fmt.Errorf("parsing hunk header %q: %w", line, err)
		}

		lineNum := hunk.NewStartLine
		i++

		for i < len(lines) && !strings.HasPrefix(lines[i], diffGitPrefix) && !strings.HasPrefix(lines[i], hunkPrefix) {
			raw := lines[i]

			if raw == `\ No newline at end of file` {
				i++
				continue
			}

			if len(raw) == 0 {
				hunk.Lines = append(hunk.Lines, Line{Number: lineNum, Kind: KindContext, Content: ""})
				lineNum++
				i++
				continue
			}

			switch raw[0] {
			case '+':
				hunk.Lines = append(hunk.Lines, Line{Number: lineNum, Kind: KindAdded, Content: raw[1:]})
				lineNum++
			case '-':
				hunk.Lines = append(hunk.Lines, Line{Number: lineNum, Kind: KindRemoved, Content: raw[1:]})
			default:
				hunk.Lines = append(hunk.Lines, Line{Number: lineNum, Kind: KindContext, Content: trimLeadingSpace(raw)})
				lineNum++
			}

			i++
		}

		current.Hunks = append(current.Hunks, hunk)
		i--
	}

	return files, nil
}

func extractPath(gitDiffLine string) string {
	trimmed := strings.TrimPrefix(gitDiffLine, diffGitPrefix)
	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) < 2 {
		return strings.TrimPrefix(trimmed, "a/")
	}
	return strings.TrimPrefix(parts[1], "b/")
}

func resolvePathFromHeaders(lines []string, start int, fallback string) string {
	for j := start + 1; j < len(lines) && j <= start+4; j++ {
		if strings.HasPrefix(lines[j], diffGitPrefix) || strings.HasPrefix(lines[j], hunkPrefix) {
			break
		}

		if strings.HasPrefix(lines[j], "+++ ") {
			target := strings.TrimPrefix(lines[j], "+++ ")
			if target == devNull {
				return fallback
			}
			return strings.TrimPrefix(target, "b/")
		}
	}
	return fallback
}

// parseHunkHeader parses "@@ -old,count +new,count @@" into a Hunk.
func parseHunkHeader(line string) (Hunk, error) {
	// Find the range portion between the @@ markers
	// Format: @@ -old_start[,old_count] +new_start[,new_count] @@
	trimmed := strings.TrimPrefix(line, "@@ ")
	if end := strings.Index(trimmed, " @@"); end >= 0 {
		trimmed = trimmed[:end]
	}

	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}

	oldRange := strings.TrimPrefix(parts[0], "-")
	newRange := strings.TrimPrefix(parts[1], "+")

	oldStart, oldCount, err := parseRange(oldRange)
	if err != nil {
		return Hunk{}, fmt.Errorf("parsing old range %q: %w", oldRange, err)
	}

	newStart, newCount, err := parseRange(newRange)
	if err != nil {
		return Hunk{}, fmt.Errorf("parsing new range %q: %w", newRange, err)
	}

	return Hunk{
		OldStartLine: oldStart,
		OldLineCount: oldCount,
		NewStartLine: newStart,
		NewLineCount: newCount,
	}, nil
}

func parseRange(s string) (int, int, error) {
	parts := strings.SplitN(s, ",", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing start: %w", err)
	}

	count := 1
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("parsing count: %w", err)
		}
	}

	return start, count, nil
}

func trimLeadingSpace(s string) string {
	if len(s) > 0 && s[0] == ' ' {
		return s[1:]
	}
	return s
}
