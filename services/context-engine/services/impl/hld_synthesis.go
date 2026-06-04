package impl

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go"
	openaiopt "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

const synthesisTimeout = 30 * time.Second
const synthesisMaxOutputTokens = 400
const synthesisFileCap = 25

const synthesisSystemPrompt = `You are SuperCoder's HLD synthesizer. Given per-chunk review notes and a list of findings from a multi-chunk PR review, produce ONE overview and ONE risk_reason for the whole PR.

Output ONLY two tags, in this order:
<overview>One sentence: what this PR does, factual.</overview>
<risk_reason>One sentence: what concrete risks the findings expose. Name the categories (e.g. "tenant isolation gap", "concurrent S3 race", "missing transaction"). If no findings, describe the blast radius briefly.</risk_reason>

Rules:
- Be specific. Reference actual components or behaviour, not vague phrases like "the code" or "the system".
- No preamble, no markdown headers, no closing remarks. Just the two tags.
- Each tag's content must be a single line, no newlines.`

// synthesizeHLD folds per-chunk overviews / risk reasons into a whole-PR pair via one small LLM call.
// Best-effort: returns ("", "") on error; adds tokens to metrics when non-nil.
func (p *Pipeline) synthesizeHLD(
	ctx context.Context,
	files []services.PRFileInfo,
	chunkOverviews []string,
	chunkRiskReasons []string,
	issues []services.ReviewIssue,
	metrics *services.ReviewMetrics,
) (overview, riskReason string) {
	user := buildSynthesisUserPrompt(files, chunkOverviews, chunkRiskReasons, issues)

	timeoutCtx, cancel := context.WithTimeout(ctx, synthesisTimeout)
	defer cancel()

	var (
		raw     string
		err     error
		inTok   int
		outTok  int
		modelID string
	)
	switch p.reviewerCfg.LLMProvider() {
	case "openai":
		modelID = p.openAICfg.Model()
		raw, inTok, outTok, err = oneShotOpenAI(
			timeoutCtx,
			p.openAICfg.APIKey(),
			modelID,
			synthesisSystemPrompt,
			user,
			synthesisMaxOutputTokens,
		)
	default:
		modelID = p.anthropicCfg.Model()
		raw, inTok, outTok, err = oneShotAnthropic(
			timeoutCtx,
			p.anthropicCfg.APIKey(),
			modelID,
			synthesisSystemPrompt,
			user,
			synthesisMaxOutputTokens,
		)
	}
	if err != nil {
		p.logger.Warn("HLD synthesis failed, falling back to empty fields", zap.Error(err))
		return "", ""
	}

	if metrics != nil {
		metrics.InputTokens += inTok
		metrics.OutputTokens += outTok
		metrics.TotalTokens += inTok + outTok
	}

	overview = singleLine(extractSynthTag(raw, "overview"))
	riskReason = singleLine(extractSynthTag(raw, "risk_reason"))
	p.logger.Info("HLD synthesized",
		zap.String("model", modelID),
		zap.Int("input_tokens", inTok),
		zap.Int("output_tokens", outTok),
		zap.Int("overview_chars", len(overview)),
		zap.Int("risk_reason_chars", len(riskReason)))
	return overview, riskReason
}

func buildSynthesisUserPrompt(
	files []services.PRFileInfo,
	chunkOverviews, chunkRiskReasons []string,
	issues []services.ReviewIssue,
) string {
	var sb strings.Builder

	sb.WriteString("Files in this PR:\n")
	limit := len(files)
	if limit > synthesisFileCap {
		limit = synthesisFileCap
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		sb.WriteString(fmt.Sprintf("- %s (+%d -%d, %s)\n", f.Filename, f.Additions, f.Deletions, f.Status))
	}
	if len(files) > synthesisFileCap {
		sb.WriteString(fmt.Sprintf("...and %d more files\n", len(files)-synthesisFileCap))
	}

	if len(chunkOverviews) > 0 {
		sb.WriteString("\nPer-chunk overviews:\n")
		for i, ov := range chunkOverviews {
			sb.WriteString(fmt.Sprintf("- chunk %d: %s\n", i+1, singleLine(ov)))
		}
	}

	if len(chunkRiskReasons) > 0 {
		sb.WriteString("\nPer-chunk risk reasons:\n")
		for i, rr := range chunkRiskReasons {
			sb.WriteString(fmt.Sprintf("- chunk %d: %s\n", i+1, singleLine(rr)))
		}
	}

	if len(issues) > 0 {
		type bucket struct{ high, medium, low int }
		byCat := make(map[string]*bucket)
		for _, iss := range issues {
			b := byCat[iss.Category]
			if b == nil {
				b = &bucket{}
				byCat[iss.Category] = b
			}
			switch iss.Severity {
			case "high":
				b.high++
			case "medium":
				b.medium++
			default:
				b.low++
			}
		}
		cats := make([]string, 0, len(byCat))
		for c := range byCat {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		sb.WriteString("\nFindings by category:\n")
		for _, c := range cats {
			b := byCat[c]
			sb.WriteString(fmt.Sprintf("- %s: %d high, %d medium, %d low\n", c, b.high, b.medium, b.low))
		}
	} else {
		sb.WriteString("\nNo findings reported.\n")
	}

	sb.WriteString("\nProduce <overview> and <risk_reason> for the entire PR now.")
	return sb.String()
}

func oneShotAnthropic(
	ctx context.Context,
	apiKey, model, systemPrompt, userPrompt string,
	maxTokens int,
) (string, int, int, error) {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	client := anthropic.NewClient(opts...)

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", 0, 0, err
	}

	var out strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	return out.String(), int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), nil
}

func oneShotOpenAI(
	ctx context.Context,
	apiKey, model, systemPrompt, userPrompt string,
	maxTokens int,
) (string, int, int, error) {
	opts := []openaiopt.RequestOption{}
	if apiKey != "" {
		opts = append(opts, openaiopt.WithAPIKey(apiKey))
	}
	client := openai.NewClient(opts...)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Model:           model,
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(userPrompt)},
		Instructions:    openai.String(systemPrompt),
		MaxOutputTokens: openai.Int(int64(maxTokens)),
	})
	if err != nil {
		return "", 0, 0, err
	}
	if resp.Status == responses.ResponseStatusFailed {
		return "", 0, 0, fmt.Errorf("openai response failed: %s", resp.Error.Message)
	}
	return resp.OutputText(), int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), nil
}

// extractSynthTag returns the first matching tag's trimmed content.
func extractSynthTag(raw, tag string) string {
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
	if m := re.FindStringSubmatch(raw); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// singleLine collapses whitespace runs to single spaces. The HLD renders overview/risk on one line.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
