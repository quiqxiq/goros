package ssh

import "strings"

// CleanConsoleOutput ports cleanConsoleOutput from the centrs reference
// project (src/protocols/ssh.ts L182): normalize CRLF to LF, trim trailing
// whitespace per line, and drop blank leading/trailing lines. Leading
// indentation of content lines is preserved (print output). Interior blank
// lines are kept.
func CleanConsoleOutput(stdout string) string {
	normalized := strings.ReplaceAll(stdout, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(strings.TrimRight(lines[i], " \t"))
		if i < end-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
