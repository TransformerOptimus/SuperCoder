package mappers

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
)

// Precompiled regex patterns for review output parsing.
var (
	reviewBlockRe = regexp.MustCompile(`(?s)<review>(.*)</review>`)
	issueBlockRe  = regexp.MustCompile(`(?s)<issue>(.*?)</issue>`)
	verdictXMLRe  = regexp.MustCompile(`(?s)<verdict>\s*(APPROVE|REQUEST_CHANGES|NEEDS_DISCUSSION)\s*</verdict>`)

	issueHeaderRe = regexp.MustCompile(`###\s*[^\[]*\[([^\]]+)\]:\s*(.+)`)
	fieldRe       = regexp.MustCompile(`\*\*(\w+):\*\*\s*(.+)`)
	verdictMDRe   = regexp.MustCompile(`(?i)VERDICT:\s*(APPROVE|REQUEST_CHANGES|NEEDS_DISCUSSION)`)

	mdSeparatorRe = regexp.MustCompile(`(?m)^---\s*$`)

	// Precompiled tag extraction regexes to avoid recompiling on each call.
	tagRegexes = map[string]*regexp.Regexp{
		"file":        regexp.MustCompile(`(?s)<file>(.*?)</file>`),
		"line":        regexp.MustCompile(`(?s)<line>(.*?)</line>`),
		"lines":       regexp.MustCompile(`(?s)<lines>(.*?)</lines>`),
		"severity":    regexp.MustCompile(`(?s)<severity>(.*?)</severity>`),
		"category":    regexp.MustCompile(`(?s)<category>(.*?)</category>`),
		"description": regexp.MustCompile(`(?s)<description>(.*?)</description>`),
		"problem":     regexp.MustCompile(`(?s)<problem>(.*?)</problem>`),
		"fix":         regexp.MustCompile(`(?s)<fix>(.*?)</fix>`),
	}
)

// ParseReviewOutput parses the agent's raw output into a structured ReviewResponse.
func ParseReviewOutput(raw string) *dto.ReviewResponse {
	resp := &dto.ReviewResponse{
		Comments:  []dto.ReviewCommentResponse{},
		RawOutput: raw,
		Metrics: dto.ReviewMetrics{
			SeverityCounts: make(map[string]int),
			CategoryCounts: make(map[string]int),
		},
	}

	if reviewMatch := reviewBlockRe.FindStringSubmatch(raw); reviewMatch != nil {
		reviewBody := reviewMatch[1]
		resp.Comments = parseXMLIssues(reviewBody)
		resp.Verdict = parseXMLVerdict(reviewBody)
	} else {
		resp.Comments = parseMDIssueBlocks(raw)
		resp.Verdict = parseMDVerdict(raw)
	}

	for _, c := range resp.Comments {
		resp.Metrics.SeverityCounts[c.Severity]++
		resp.Metrics.CategoryCounts[c.Category]++
	}
	resp.Metrics.CommentCount = len(resp.Comments)

	return resp
}

func extractXMLTag(block, tag string) string {
	re, ok := tagRegexes[tag]
	if !ok {
		re = regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
	}
	match := re.FindStringSubmatch(block)
	if match != nil {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func parseXMLIssues(reviewBody string) []dto.ReviewCommentResponse {
	var comments []dto.ReviewCommentResponse

	matches := issueBlockRe.FindAllStringSubmatch(reviewBody, -1)
	for _, m := range matches {
		issueBody := m[1]

		lineNum := parseLineNumber(extractXMLTag(issueBody, "line"))
		if lineNum == nil {
			lineNum = parseFirstLineNumber(extractXMLTag(issueBody, "lines"))
		}

		comment := dto.ReviewCommentResponse{
			FilePath:   extractXMLTag(issueBody, "file"),
			LineNumber: lineNum,
			Severity:   strings.ToLower(extractXMLTag(issueBody, "severity")),
			Category:   extractXMLTag(issueBody, "category"),
			Body: formatCommentBody(
				extractXMLTag(issueBody, "description"),
				extractXMLTag(issueBody, "problem"),
				extractXMLTag(issueBody, "fix"),
			),
		}
		if comment.Category != "" || comment.Body != "" {
			comments = append(comments, comment)
		}
	}

	return comments
}

func parseXMLVerdict(reviewBody string) string {
	match := verdictXMLRe.FindStringSubmatch(reviewBody)
	if match != nil {
		return strings.ToUpper(strings.TrimSpace(match[1]))
	}
	return ""
}

func parseMDIssueBlocks(raw string) []dto.ReviewCommentResponse {
	var comments []dto.ReviewCommentResponse

	blocks := mdSeparatorRe.Split(raw, -1)

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		headerMatch := issueHeaderRe.FindStringSubmatch(block)
		if headerMatch == nil {
			continue
		}

		comment := dto.ReviewCommentResponse{
			Category: strings.TrimSpace(headerMatch[1]),
			Body:     strings.TrimSpace(headerMatch[2]),
		}

		fieldMatches := fieldRe.FindAllStringSubmatch(block, -1)
		for _, fm := range fieldMatches {
			key := strings.ToLower(strings.TrimSpace(fm[1]))
			val := strings.Trim(strings.TrimSpace(fm[2]), "'`\"")
			switch key {
			case "file":
				comment.FilePath = val
			case "severity":
				comment.Severity = val
			}
		}

		comments = append(comments, comment)
	}

	return comments
}

func parseMDVerdict(raw string) string {
	match := verdictMDRe.FindStringSubmatch(raw)
	if match != nil {
		return strings.ToUpper(strings.TrimSpace(match[1]))
	}
	return ""
}

func parseLineNumber(lineStr string) *int {
	if lineStr == "" {
		return nil
	}
	p := strings.TrimSpace(lineStr)
	if n, err := strconv.Atoi(p); err == nil && n > 0 {
		return &n
	}
	return nil
}

func parseFirstLineNumber(lineStr string) *int {
	if lineStr == "" {
		return nil
	}
	cleaned := strings.ReplaceAll(lineStr, "L", "")
	parts := strings.Split(cleaned, "-")
	p := strings.TrimSpace(parts[0])
	if n, err := strconv.Atoi(p); err == nil {
		return &n
	}
	return nil
}

func formatCommentBody(description, problem, fix string) string {
	var parts []string
	if description != "" {
		parts = append(parts, "**"+description+"**")
	}
	if problem != "" {
		parts = append(parts, problem)
	}
	if fix != "" {
		parts = append(parts, "> **Suggested fix:** "+fix)
	}
	return strings.Join(parts, "\n\n")
}
