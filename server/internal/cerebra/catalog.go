package cerebra

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	paramSizeRegex    = regexp.MustCompile(`(?i)(?:^|[-_.:/@ ])(\d+(?:\.\d+)?)\s*([bmk])(?:$|[-_.:/@ ])`)
	dateSuffixRegex   = regexp.MustCompile(`(?i)(?:^|[-_.:/@ ])(\d{4,8}|\d{2}-\d{4})(?:$|[-_.:/@ ])`)
	modelVersionRegex = regexp.MustCompile(`(?i)(?:^|[-_./:@ ])v?(\d+(?:\.\d+)?)(?:$|[-_./:@ ])`)
)

// extractParamSizeInBillions parses parameter size from model names (e.g. "qwen2.5:0.5b" -> 0.5, "llama3.3:70b" -> 70, "smollm:135m" -> 0.135).
func extractParamSizeInBillions(name string) (float64, bool) {
	matches := paramSizeRegex.FindStringSubmatch(name)
	if len(matches) < 3 {
		return 0, false
	}
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.ToLower(matches[2])
	switch unit {
	case "m":
		return val / 1000.0, true
	case "k":
		return val / 1000000.0, true
	case "b":
		return val, true
	default:
		return val, true
	}
}

// extractModelVersion extracts the generation/version number of an LLM (e.g. "gemini-3.8-flash" -> 3.8, "claude-3-7-sonnet" -> 3.7).
func extractModelVersion(name string) float64 {
	base := getBaseModelName(name)
	// Strip date stamps (e.g. 20241022, 04-2026) so dates aren't parsed as versions.
	cleaned := dateSuffixRegex.ReplaceAllString(base, " ")
	// Strip parameter sizes (e.g. 70b, 32b) so param sizes aren't parsed as versions.
	cleaned = paramSizeRegex.ReplaceAllString(cleaned, " ")

	matches := modelVersionRegex.FindAllStringSubmatch(cleaned, -1)
	maxVer := 0.0
	for _, m := range matches {
		if len(m) >= 2 {
			if ver, err := strconv.ParseFloat(m[1], 64); err == nil {
				if ver > maxVer && ver < 25.0 {
					maxVer = ver
				}
			}
		}
	}
	return maxVer
}

// selectHighestVersionModel picks the model with the highest version number from a candidate list,
// breaking ties by preserving candidate order.
func selectHighestVersionModel(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	best := candidates[0]
	bestVer := extractModelVersion(best)

	for _, c := range candidates[1:] {
		ver := extractModelVersion(c)
		if ver > bestVer {
			best = c
			bestVer = ver
		}
	}
	return best
}

// ModelProfile contains metadata and capability classification for a discovered model.
type ModelProfile struct {
	ModelID string
	Tier    Tier
	Score   int
}

// isValidModelIdentifier validates that a model ID does not contain control characters
// and is within a safe length bound (max 256 characters).
func isValidModelIdentifier(model string) bool {
	if len(model) == 0 || len(model) > 256 {
		return false
	}
	for _, r := range model {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// ClassifyModelTier analyzes a model name/identifier and assigns its optimal complexity tier
// based on capability, latency, and parameter tier heuristics.
func ClassifyModelTier(modelID string) Tier {
	if !isValidModelIdentifier(modelID) {
		return TierStandard
	}
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return TierStandard
	}

	// Strip provider prefix (e.g. "opencode/", "anthropic/", "openai/", "ollama/") for model name matching.
	baseName := lower
	if idx := strings.LastIndex(lower, "/"); idx != -1 {
		baseName = lower[idx+1:]
	}

	// Universal Simple / Fast capability descriptors:
	// Mini, nano, lite, and haiku models are always Simple Tier (even when accompanied by version numbers like 4.5)
	if hasModelSegment(baseName, "mini") || hasModelSegment(baseName, "nano") || hasModelSegment(baseName, "haiku") || hasModelSegment(baseName, "lite") {
		return TierSimple
	}

	// 1. Universal Heavy / Frontier Reasoning descriptors:
	heavyKeywords := []string{
		"reasoning", "ultra", "pro", "max", "large", "opus", "r1", "o1", "o3", "pickle", "4.5", "frontier",
	}
	for _, kw := range heavyKeywords {
		if hasModelSegment(baseName, kw) {
			return TierHeavy
		}
	}

	// 2. Universal Simple / Fast capability descriptors:
	simpleKeywords := []string{
		"small", "tiny", "mimo", "spark", "hy3", "preview",
	}
	for _, kw := range simpleKeywords {
		if hasModelSegment(baseName, kw) {
			return TierSimple
		}
	}

	// Modern flash models without lite:
	// Flagship flash models (v3.0+, like Gemini 3.5, 3.6, 3.7, 3.8 Flash) are premier coding/instruct workhorses -> Standard Tier.
	// Sub-3.0 flash models (e.g. 1.5, 2.0, 2.5 flash) -> Simple Tier.
	if hasModelSegment(baseName, "flash") {
		if ver := extractModelVersion(baseName); ver >= 3.0 {
			return TierStandard
		}
		return TierSimple
	}

	// 3. Universal Standard Coding / Instruct descriptors:
	standardKeywords := []string{
		"coder", "code", "instruct", "sonnet", "standard", "chat", "lightning",
	}
	for _, kw := range standardKeywords {
		if hasModelSegment(baseName, kw) {
			return TierStandard
		}
	}

	// 4. Dynamic Parameter-Size Auto-Detection (0.5B, 1B, 7B, 14B, 32B, 70B, etc.):
	if size, ok := extractParamSizeInBillions(baseName); ok {
		if size <= 4.0 {
			// <= 4B parameters (e.g. 0.5B, 1B, 1.5B, 2B, 3B) -> Simple Tier
			return TierSimple
		} else if size >= 30.0 {
			// >= 30B parameters (e.g. 31B, 32B, 70B, 405B) -> Heavy Tier
			return TierHeavy
		} else {
			// 4B to 30B parameters (e.g. 7B, 8B, 9B, 12B, 14B) -> Standard Tier
			return TierStandard
		}
	}

	// Default to Standard Tier
	return TierStandard
}

// hasModelSegment checks whether keyword exists in name bounded by delimiters (-_./ : or start/end).
func hasModelSegment(name, keyword string) bool {
	if keyword == "" {
		return false
	}
	offset := 0
	for {
		idx := strings.Index(name[offset:], keyword)
		if idx == -1 {
			return false
		}
		absIdx := offset + idx
		end := absIdx + len(keyword)

		beforeOK := absIdx == 0 || isSegmentDelimiter(name[absIdx-1])
		afterOK := end == len(name) || isSegmentDelimiter(name[end])

		if beforeOK && afterOK {
			return true
		}
		offset = absIdx + 1
		if offset >= len(name) {
			return false
		}
	}
}

func isSegmentDelimiter(b byte) bool {
	return b == '-' || b == '_' || b == '.' || b == '/' || b == ':' || b == ' ' || b == '@'
}

func isNonChatModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if lower == "auto" || lower == "default" || lower == "none" {
		return true
	}
	nonChatTags := []string{"embedding", "embed", "tts", "-image", "-video", "audio", "lyria", "veo", "research", "clip", "translate", "robotics", "customtools"}
	for _, tag := range nonChatTags {
		if strings.Contains(lower, tag) {
			return true
		}
	}
	if size, ok := extractParamSizeInBillions(lower); ok && size < 0.2 {
		return true // Sub-200M models lack capacity for agent system prompts & tool calling;
		// models like qwen2.5:0.5b and qwen3:0.6b are capable tool-callers and must not be excluded
	}
	return false
}

// BuildTierMapFromCatalog scans a slice of discovered runtime models and dynamically selects
// the optimal model for each of the 3 tiers (Simple, Standard, Heavy).
func BuildTierMapFromCatalog(availableModels []string) TierMap {
	tierMap := make(TierMap)

	if len(availableModels) == 0 {
		return tierMap
	}

	var simpleCandidates []string
	var standardCandidates []string
	var heavyCandidates []string

	for _, model := range availableModels {
		if strings.TrimSpace(model) == "" || !isValidModelIdentifier(model) || isNonChatModel(model) {
			continue
		}
		tier := ClassifyModelTier(model)
		switch tier {
		case TierSimple:
			simpleCandidates = append(simpleCandidates, model)
		case TierStandard:
			standardCandidates = append(standardCandidates, model)
		case TierHeavy:
			heavyCandidates = append(heavyCandidates, model)
		}
	}

	// 1. Assign Simple Tier
	if len(simpleCandidates) > 0 {
		tierMap[TierSimple] = selectBestSimpleModel(simpleCandidates)
	} else if len(standardCandidates) > 0 {
		tierMap[TierSimple] = selectBestSimpleModel(standardCandidates)
	} else if len(heavyCandidates) > 0 {
		tierMap[TierSimple] = heavyCandidates[0]
	}

	// 2. Assign Standard Tier
	if len(standardCandidates) > 0 {
		tierMap[TierStandard] = selectBestStandardModel(standardCandidates)
	} else if len(heavyCandidates) > 0 {
		tierMap[TierStandard] = heavyCandidates[0]
	} else if len(simpleCandidates) > 0 {
		tierMap[TierStandard] = simpleCandidates[0]
	}

	// 3. Assign Heavy Tier
	if len(heavyCandidates) > 0 {
		tierMap[TierHeavy] = selectBestHeavyModel(heavyCandidates)
	} else if len(standardCandidates) > 0 {
		tierMap[TierHeavy] = selectBestHeavyModel(standardCandidates)
	} else if len(simpleCandidates) > 0 {
		tierMap[TierHeavy] = simpleCandidates[0]
	}

	return tierMap
}

// filterPreferredPool separates candidates into two buckets:
//   - direct: any runtime or provider where the user has a direct connection
//     (opencode, claude, kimi, grok, openclaw, qoder, hermes, or any other runtime)
//   - proxy: public aggregator proxies (openrouter) which are last resort
//
// Local Ollama is treated as a direct provider — it is on the user's machine and
// requires no external network. All direct providers are returned together with no
// further ranking between them; the tier-selection functions (selectBestSimpleModel,
// selectBestStandardModel, selectBestHeavyModel) pick the best within that pool by
// capability heuristics, which is the right signal regardless of runtime name.
func filterPreferredPool(candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}

	var direct []string
	var proxy []string

	for _, c := range candidates {
		lower := strings.ToLower(c)
		// OpenRouter is a public proxy aggregator — treat as last resort.
		// Any other prefix (opencode/, claude/, kimi/, grok/, openclaw/, ollama/, google/,
		// anthropic/, openai/, or any custom runtime) is a direct provider.
		if strings.HasPrefix(lower, "openrouter/") || strings.Contains(lower, "/openrouter/") {
			proxy = append(proxy, c)
		} else {
			direct = append(direct, c)
		}
	}

	if len(direct) > 0 {
		return direct
	}
	return proxy
}

func getBaseModelName(model string) string {
	lower := strings.ToLower(model)
	if idx := strings.LastIndex(lower, "/"); idx != -1 {
		return lower[idx+1:]
	}
	return lower
}

func selectBestSimpleModel(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	searchPool := filterPreferredPool(candidates)

	// 1. Prefer fast high-quality lightweight/low-level models:
	// Prioritize true "low-level" / lite / nano / mini descriptors ahead of full flash models
	simpleTags := []string{"lite", "nano", "mini", "small", "tiny", "mimo", "haiku", "flash", "spark"}
	for _, tag := range simpleTags {
		var matches []string
		for _, c := range searchPool {
			base := getBaseModelName(c)
			if hasModelSegment(base, tag) {
				matches = append(matches, c)
			}
		}
		if len(matches) > 0 {
			return selectHighestVersionModel(matches)
		}
	}

	// 2. Prefer 3B to 8B lightweight models
	var paramMatches []string
	for _, c := range searchPool {
		if size, ok := extractParamSizeInBillions(c); ok && size >= 3.0 && size <= 8.0 {
			paramMatches = append(paramMatches, c)
		}
	}
	if len(paramMatches) > 0 {
		return selectHighestVersionModel(paramMatches)
	}
	return selectHighestVersionModel(searchPool)
}

func selectBestStandardModel(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	searchPool := filterPreferredPool(candidates)

	// 1. Prefer balanced coding / instruct / flagship flash model indicators in priority order.
	// Tag-based matching runs BEFORE param-size so that named cloud models
	// (e.g. opencode/nemotron-3.5-lightning-free, gemini-3.8-flash) beat local Ollama models
	// that happen to fall in the 7B-16B size window.
	standardTags := []string{"sonnet", "coder", "flash", "lightning", "instruct", "code", "standard"}
	for _, tag := range standardTags {
		var matches []string
		for _, c := range searchPool {
			// Skip ollama models in tag pass — prefer cloud provider models first
			if strings.HasPrefix(strings.ToLower(c), "ollama/") {
				continue
			}
			base := getBaseModelName(c)
			if hasModelSegment(base, tag) {
				matches = append(matches, c)
			}
		}
		if len(matches) > 0 {
			return selectHighestVersionModel(matches)
		}
	}

	// 2. Prefer mid-sized coding parameter models (7B - 16B) — applies to
	// local Ollama models or any provider without a matching tag above.
	var paramMatches []string
	for _, c := range searchPool {
		if size, ok := extractParamSizeInBillions(c); ok && size >= 7.0 && size <= 16.0 {
			paramMatches = append(paramMatches, c)
		}
	}
	if len(paramMatches) > 0 {
		return selectHighestVersionModel(paramMatches)
	}

	// 3. Tag pass again but now include ollama models (cloud exhausted)
	for _, tag := range standardTags {
		var matches []string
		for _, c := range searchPool {
			base := getBaseModelName(c)
			if hasModelSegment(base, tag) {
				matches = append(matches, c)
			}
		}
		if len(matches) > 0 {
			return selectHighestVersionModel(matches)
		}
	}

	// 4. Fall back to local Ollama models if present
	for _, c := range searchPool {
		if strings.HasPrefix(strings.ToLower(c), "ollama/") {
			return c
		}
	}

	return selectHighestVersionModel(searchPool)
}

func selectBestHeavyModel(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	searchPool := filterPreferredPool(candidates)

	// 1. Prefer frontier reasoning tags in priority order (ultra, pro, opus, r1, o1, o3, max, large, reasoning)
	heavyTags := []string{"ultra", "pro", "opus", "r1", "o1", "o3", "reasoning", "max", "large", "pickle"}
	for _, tag := range heavyTags {
		var matches []string
		for _, c := range searchPool {
			base := getBaseModelName(c)
			if hasModelSegment(base, tag) {
				matches = append(matches, c)
			}
		}
		if len(matches) > 0 {
			return selectHighestVersionModel(matches)
		}
	}

	// 2. Prefer large parameter models (>= 30B, e.g. 31B, 32B, 70B, 405B)
	var paramMatches []string
	for _, c := range searchPool {
		if size, ok := extractParamSizeInBillions(c); ok && size >= 30.0 {
			paramMatches = append(paramMatches, c)
		}
	}
	if len(paramMatches) > 0 {
		return selectHighestVersionModel(paramMatches)
	}

	return selectHighestVersionModel(searchPool)
}
