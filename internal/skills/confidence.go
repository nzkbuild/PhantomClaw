package skills

// Confidence scoring gate — every trade must pass this before execution (PRD §14).
// MVP uses 3 factors. Phase 2 adds COT, echo recall, spread, news (7 total).

// ConfidenceInput holds the data needed for scoring.
type ConfidenceInput struct {
	// Factor 1: MTF Alignment (0-25 pts)
	// Do D1, H4, and H1 agree on direction?
	D1Bias string // "bullish" | "bearish" | "neutral"
	H4Bias string
	H1Bias string

	// Factor 2: S/R Confluence (0-20 pts)
	// Is the entry near a key support/resistance level?
	NearKeyLevel     bool
	KeyLevelDistance float64 // Distance in pips to nearest S/R
	KeyLevelStrength int     // 1-3 (how many times price bounced)

	// Factor 3: Session Quality (0-15 pts)
	// Is this a high-liquidity session?
	Session string // "LONDON" | "NY" | "OVERLAP" | "TOKYO" | "OFF"
}

// ConfidenceResult holds the scoring output.
type ConfidenceResult struct {
	Score     int            // 0-100 total
	Action    string         // "EXECUTE" | "REDUCED" | "SKIP" | "HARD_SKIP"
	LotFactor float64        // 1.0 = full, 0.5 = reduced, 0.0 = skip
	Breakdown map[string]int // Factor → points
}

// ScoreConfidence calculates the trade confidence score (MVP: 3 factors, max 60 pts scaled to 100).
func ScoreConfidence(input ConfidenceInput) ConfidenceResult {
	breakdown := make(map[string]int)
	total := 0

	// Factor 1: MTF Alignment (0-25 pts)
	mtfScore := scoreMTFAlignment(input.D1Bias, input.H4Bias, input.H1Bias)
	breakdown["mtf_alignment"] = mtfScore
	total += mtfScore

	// Factor 2: S/R Confluence (0-20 pts)
	srScore := scoreSRConfluence(input.NearKeyLevel, input.KeyLevelDistance, input.KeyLevelStrength)
	breakdown["sr_confluence"] = srScore
	total += srScore

	// Factor 3: Session Quality (0-15 pts)
	sessionScore := scoreSessionQuality(input.Session)
	breakdown["session_quality"] = sessionScore
	total += sessionScore

	// MVP: 3 factors max = 60 pts. Scale to 0-100 for consistent thresholds.
	// In Phase 2, when all 7 factors added, remove scaling.
	scaled := int(float64(total) / 60.0 * 100.0)
	if scaled > 100 {
		scaled = 100
	}

	result := ConfidenceResult{
		Score:     scaled,
		Breakdown: breakdown,
	}

	// Apply threshold logic (PRD §14)
	switch {
	case scaled >= 80:
		result.Action = "EXECUTE"
		result.LotFactor = 1.0
	case scaled >= 60:
		result.Action = "REDUCED"
		result.LotFactor = 0.5
	case scaled >= 40:
		result.Action = "SKIP"
		result.LotFactor = 0.0
	default:
		result.Action = "HARD_SKIP"
		result.LotFactor = 0.0
	}

	return result
}

// scoreMTFAlignment: 25 pts max.
// All 3 TFs agree = 25, D1+H4 agree = 18, D1+H1 agree = 15, only 1 TF = 8, none = 0.
func scoreMTFAlignment(d1, h4, h1 string) int {
	if d1 == "" || d1 == "neutral" {
		d1 = "neutral"
	}
	if h4 == "" || h4 == "neutral" {
		h4 = "neutral"
	}
	if h1 == "" || h1 == "neutral" {
		h1 = "neutral"
	}

	allAgree := d1 != "neutral" && d1 == h4 && h4 == h1
	d1h4 := d1 != "neutral" && d1 == h4
	d1h1 := d1 != "neutral" && d1 == h1
	h4h1 := h4 != "neutral" && h4 == h1

	switch {
	case allAgree:
		return 25
	case d1h4:
		return 18
	case d1h1:
		return 15
	case h4h1:
		return 12
	case d1 != "neutral":
		return 8
	case h4 != "neutral":
		return 6
	case h1 != "neutral":
		return 4
	default:
		return 0
	}
}

// scoreSRConfluence: 20 pts max.
func scoreSRConfluence(nearLevel bool, distance float64, strength int) int {
	if !nearLevel {
		return 0
	}

	score := 0

	// Distance: closer = better
	switch {
	case distance <= 5:
		score += 12
	case distance <= 15:
		score += 8
	case distance <= 30:
		score += 4
	default:
		return 0
	}

	// Strength: 3 bounces > 2 > 1
	switch {
	case strength >= 3:
		score += 8
	case strength >= 2:
		score += 5
	case strength >= 1:
		score += 3
	}

	if score > 20 {
		score = 20
	}
	return score
}

// scoreSessionQuality: 15 pts max.
func scoreSessionQuality(session string) int {
	switch session {
	case "OVERLAP":
		return 15 // London/NY overlap — best liquidity
	case "LONDON":
		return 12
	case "NY":
		return 10
	case "TOKYO":
		return 5
	default:
		return 0 // OFF hours
	}
}
