package impl

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// Risk weights sum to 1.0.
const (
	weightBlastRadius   = 0.35
	weightContractDiffs = 0.30
	weightDiffSize      = 0.35
)

// Caps normalise raw values into [0, 1] — anything above saturates the factor.
const (
	capBlastRadius   = 20
	capContractDiffs = 5
	capDiffSize      = 500
)

// RiskAssessment is the deterministic structural-risk output.
type RiskAssessment struct {
	Level   string
	Score   float64
	Reason  string
	Factors []services.RiskFactor
}

// computeRisk produces a level + score from blast radius, contract diffs, and diff size.
func computeRisk(
	files []services.PRFileInfo,
	graphCtx *GraphContext,
	contractDiffs []ContractDiff,
) RiskAssessment {
	maxCallers, totalCallers := blastRadiusStats(graphCtx)
	additions := totalAdditions(files)

	brValue := normalizedRatio(float64(maxCallers), capBlastRadius)
	cdValue := normalizedRatio(float64(len(contractDiffs)), capContractDiffs)
	dsValue := normalizedRatio(float64(additions), capDiffSize)

	score := brValue*weightBlastRadius + cdValue*weightContractDiffs + dsValue*weightDiffSize
	level := levelFromScore(score)

	factors := []services.RiskFactor{
		{
			Name:   "blast_radius",
			Weight: weightBlastRadius,
			Value:  round2(brValue),
			Detail: blastDetail(graphCtx, maxCallers, totalCallers),
		},
		{
			Name:   "contract_diffs",
			Weight: weightContractDiffs,
			Value:  round2(cdValue),
			Detail: contractDetail(contractDiffs),
		},
		{
			Name:   "diff_size",
			Weight: weightDiffSize,
			Value:  round2(dsValue),
			Detail: fmt.Sprintf("%d additions across %d files", additions, len(files)),
		},
	}
	return RiskAssessment{
		Level:   level,
		Score:   round2(score),
		Reason:  buildRiskReason(factors),
		Factors: factors,
	}
}

// escalateRiskWithFindings lifts the structural level when the LLM finds high-severity bugs.
func escalateRiskWithFindings(base RiskAssessment, issues []services.ReviewIssue) RiskAssessment {
	highCount := 0
	mediumCount := 0
	for _, iss := range issues {
		switch iss.Severity {
		case "high":
			highCount++
		case "medium":
			mediumCount++
		}
	}
	if highCount > 0 && base.Level != "High" {
		base.Level = "High"
		base.Reason = fmt.Sprintf("%d high-severity finding%s; %s",
			highCount, plural(highCount), base.Reason)
	} else if mediumCount >= 3 && base.Level == "Low" {
		base.Level = "Medium"
		base.Reason = fmt.Sprintf("%d medium-severity findings; %s", mediumCount, base.Reason)
	}
	return base
}

func blastRadiusStats(g *GraphContext) (maxCallers, totalCallers int) {
	if g == nil {
		return 0, 0
	}
	for _, callers := range g.CallerMap {
		if len(callers) > maxCallers {
			maxCallers = len(callers)
		}
		totalCallers += len(callers)
	}
	return
}

func totalAdditions(files []services.PRFileInfo) int {
	sum := 0
	for _, f := range files {
		sum += int(f.Additions)
	}
	return sum
}

func normalizedRatio(value, cap float64) float64 {
	if cap <= 0 {
		return 0
	}
	return math.Min(value/cap, 1)
}

func levelFromScore(score float64) string {
	switch {
	case score >= 0.6:
		return "High"
	case score >= 0.3:
		return "Medium"
	default:
		return "Low"
	}
}

func blastDetail(g *GraphContext, maxCallers, totalCallers int) string {
	if g == nil {
		return "graph unavailable"
	}
	if g.FunctionsAnalyzed == 0 {
		return "no functions analyzed"
	}
	return fmt.Sprintf("%d callers across %d changed functions (max %d)",
		totalCallers, g.FunctionsAnalyzed, maxCallers)
}

func contractDetail(diffs []ContractDiff) string {
	if len(diffs) == 0 {
		return "no signature changes"
	}
	var fields []string
	seen := map[string]bool{}
	for _, d := range diffs {
		for _, c := range d.Changes {
			if seen[c] {
				continue
			}
			seen[c] = true
			fields = append(fields, c)
		}
	}
	sort.Strings(fields)
	return fmt.Sprintf("%d signature change%s (%s)",
		len(diffs), plural(len(diffs)), strings.Join(fields, ", "))
}

func buildRiskReason(factors []services.RiskFactor) string {
	contributions := make([]services.RiskFactor, len(factors))
	copy(contributions, factors)
	sort.SliceStable(contributions, func(i, j int) bool {
		return contributions[i].Value*contributions[i].Weight > contributions[j].Value*contributions[j].Weight
	})
	var top []string
	for _, f := range contributions {
		if f.Value*f.Weight < 0.05 {
			continue
		}
		top = append(top, f.Detail)
		if len(top) == 2 {
			break
		}
	}
	if len(top) == 0 {
		return "small, isolated change"
	}
	return strings.Join(top, "; ")
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
