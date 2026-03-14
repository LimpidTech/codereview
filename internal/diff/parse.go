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

		lineNum := hunk.StartLine
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

func parseHunkHeader(line string) (Hunk, error) {
	atIdx := strings.Index(line, "+")
	if atIdx < 0 {
		return Hunk{}, fmt.Errorf("no + range in hunk header")
	}

	rest := line[atIdx+1:]
	endIdx := strings.Index(rest, " ")
	if endIdx < 0 {
		endIdx = strings.Index(rest, hunkPrefix)
	}
	if endIdx < 0 {
		endIdx = len(rest)
	}

	rangeStr := rest[:endIdx]
	parts := strings.SplitN(rangeStr, ",", 2)
	startLine, err := strconv.Atoi(parts[0])
	if err != nil {
		return Hunk{}, fmt.Errorf("parsing start line %q: %w", parts[0], err)
	}

	return Hunk{StartLine: startLine}, nil
}

func trimLeadingSpace(s string) string {
	if len(s) > 0 && s[0] == ' ' {
		return s[1:]
	}
	return s
}
