package docprose

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var inlineCodePattern = regexp.MustCompile("`[^`]*`")
var negationSectionHeadingPattern = regexp.MustCompile(`(?i)^(?:what .+ is not|non-?goals?|out of scope|anti-?patterns?)$`)
var positiveCopulaPattern = regexp.MustCompile(`(?i)\b(?:is|are|stays|remains)\s+(?:a|an|the\s+)?[[:alpha:]][[:alnum:]_-]*(?:\s+[[:alpha:]][[:alnum:]_-]*){0,4}`)
var negationCopulaPattern = regexp.MustCompile(`(?i)\b(?:is|are|stays|remains)\b.*\bnot\b`)

type Mode string

const (
	ModeTechnical Mode = "technical"
	ModePlanning  Mode = "planning"
	ModePublic    Mode = "public"
)

type Checker struct {
	mode       Mode
	rules      []ruleSpec
	vocabulary Vocabulary
}

type lineContext struct {
	suppressNegation bool
}

func NewChecker(mode Mode, vocabulary Vocabulary) (*Checker, error) {
	if mode == "" {
		mode = ModeTechnical
	}
	rules, err := loadRulePack(string(mode))
	if err != nil {
		return nil, err
	}
	defaultVocab, err := loadDefaultVocabulary()
	if err != nil {
		return nil, err
	}
	if len(vocabulary.Accept) == 0 {
		vocabulary.Accept = defaultVocab.Accept
	}
	if len(vocabulary.Reject) == 0 {
		vocabulary.Reject = defaultVocab.Reject
	}
	return &Checker{
		mode:       mode,
		rules:      rules,
		vocabulary: vocabulary,
	}, nil
}

func (c *Checker) Findings(file, text string) []Finding {
	var findings []Finding
	lines := strings.Split(text, "\n")
	inFrontmatter := false
	inFence := false
	fenceMarker := ""
	seenContent := false
	negationSuppressed := false

	for idx, raw := range lines {
		lineNo := idx + 1
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !seenContent && trimmed == "---" {
			inFrontmatter = true
			seenContent = true
			continue
		}
		seenContent = true

		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
			}
			continue
		}

		if isFenceLine(trimmed) {
			if inFence {
				if fenceMarker != "" && strings.HasPrefix(trimmed, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
				continue
			}
			inFence = true
			fenceMarker = fenceDelimiter(trimmed)
			continue
		}
		if inFence {
			continue
		}

		if level, heading, ok := markdownHeading(trimmed); ok {
			if level == 2 || level == 3 {
				negationSuppressed = negationSectionHeadingPattern.MatchString(heading)
			}
			continue
		}

		line := stripInlineCode(raw)
		line = strings.TrimSpace(line)
		if line == "" || isMarkdownStructureLine(line) {
			continue
		}

		ctx := lineContext{suppressNegation: negationSuppressed}
		for _, rule := range c.rules {
			if rule.Kind == "negation_predicate" {
				if matchesRule(line, rule, ctx) {
					findings = append(findings, Finding{
						File:          file,
						Line:          lineNo,
						RuleID:        rule.ID,
						Severity:      rule.Severity,
						Rationale:     rule.Rationale,
						SuggestedEdit: rule.SuggestedEdit,
					})
				}
				continue
			}
			if rule.Kind == "repeated_opening" {
				if repeatedOpeningKey(line) != "" {
					findings = append(findings, Finding{
						File:          file,
						Line:          lineNo,
						RuleID:        rule.ID,
						Severity:      rule.Severity,
						Rationale:     rule.Rationale,
						SuggestedEdit: rule.SuggestedEdit,
					})
				}
				continue
			}
			if matchesRule(line, rule, ctx) {
				findings = append(findings, Finding{
					File:          file,
					Line:          lineNo,
					RuleID:        rule.ID,
					Severity:      rule.Severity,
					Rationale:     rule.Rationale,
					SuggestedEdit: rule.SuggestedEdit,
				})
			}
		}

		findings = append(findings, c.vocabularyFindings(file, lineNo, line)...)
	}

	return findings
}

func (c *Checker) vocabularyFindings(file string, lineNo int, line string) []Finding {
	var findings []Finding
	lower := strings.ToLower(line)
	for _, reject := range c.vocabulary.Reject {
		if reject == "" {
			continue
		}
		if !containsWord(lower, reject) {
			continue
		}
		if c.termAccepted(reject) {
			continue
		}
		findings = append(findings, Finding{
			File:          file,
			Line:          lineNo,
			RuleID:        "prose.vocabulary.reject",
			Severity:      "warning",
			Rationale:     fmt.Sprintf("The vocabulary term %q is discouraged in default prose because it is too generic.", reject),
			SuggestedEdit: fmt.Sprintf("Replace %q with the project-specific term or concrete noun that names the actual concept.", reject),
		})
	}
	return findings
}

func (c *Checker) termAccepted(term string) bool {
	for _, accept := range c.vocabulary.Accept {
		if accept == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(accept), strings.TrimSpace(term)) {
			return true
		}
	}
	return false
}

func matchesRule(line string, rule ruleSpec, ctx lineContext) bool {
	if isMarkdownStructureLine(line) {
		return false
	}
	switch rule.ID {
	case "prose.claim.unsupported":
		return unsupportedClaim(line)
	case "prose.ai_slop.polish":
		return aiSlop(line)
	case "prose.filler.transition":
		return fillerTransition(line)
	case "prose.specificity.actor_action":
		return missingActorAction(line)
	case "prose.cost.filler":
		return tokenCost(line)
	case "prose.structure.repeated_opening":
		return repeatedOpeningKey(line) != ""
	}
	if rule.Kind == "negation_predicate" {
		if ctx.suppressNegation {
			return false
		}
		return definitionByNegation(line)
	}
	if len(rule.ContainsAny) == 0 {
		return false
	}
	lower := strings.ToLower(line)
	for _, phrase := range rule.ContainsAny {
		if phrase == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

func isMarkdownStructureLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
		return true
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		return true
	}
	if strings.Contains(trimmed, ".md") || strings.Contains(trimmed, ".yaml") || strings.Contains(trimmed, ".json") {
		return true
	}
	if strings.HasPrefix(trimmed, "- ") && len(strings.Fields(trimmed)) <= 8 {
		return true
	}
	return false
}

func unsupportedClaim(line string) bool {
	lower := strings.ToLower(line)
	if containsAny(lower, []string{
		"robust",
		"comprehensive",
		"industry-leading",
		"world-class",
		"best-in-class",
		"cutting-edge",
	}) {
		return true
	}
	if unsupportedComprehensiveClaim(lower) {
		return true
	}
	return containsAny(lower, []string{
		"experience for everyone",
		"path forward",
		"broadly useful",
		"easy to adopt",
	})
}

func unsupportedComprehensiveClaim(lower string) bool {
	if !strings.Contains(lower, "comprehensive") {
		return false
	}
	return containsAny(lower, []string{
		"comprehensive prd",
		"comprehensive implementation plan",
		"comprehensive plan",
		"comprehensive checklist",
		"comprehensive checklists",
		"comprehensive tests",
		"comprehensive documentation",
	})
}

func aiSlop(line string) bool {
	lower := strings.ToLower(line)
	return containsAny(lower, []string{
		"complex problems",
		"powerful commands",
		"powerful automation",
		"sophisticated autonomous",
		"sophisticated control flow",
		"sophisticated multi-agent",
		"productive ways",
		"true power",
	})
}

func fillerTransition(line string) bool {
	lower := strings.ToLower(line)
	return containsAny(lower, []string{
		"to be clear",
		"first, we should note",
		"in conclusion",
		"it is important to note",
		"for clarity",
		"that said",
	})
}

func missingActorAction(line string) bool {
	lower := strings.ToLower(line)
	return containsAny(lower, []string{"enables", "supports", "streamlines"})
}

func tokenCost(line string) bool {
	lower := strings.ToLower(line)
	if containsAny(lower, []string{
		"very important",
		"in order to",
		"effectively",
		"begin to",
		"make the experience better",
	}) {
		return true
	}
	return countMatches(lower, []string{"robust", "comprehensive", "smooth", "elegant", "excited", "high"}) >= 2
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func countMatches(s string, needles []string) int {
	var count int
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			count++
		}
	}
	return count
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func markdownHeading(line string) (level int, heading string, ok bool) {
	if !strings.HasPrefix(line, "#") {
		return 0, "", false
	}
	level = 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level < 2 || level > 3 {
		return 0, "", false
	}
	if level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	heading = strings.TrimSpace(line[level+1:])
	heading = strings.TrimRightFunc(heading, func(r rune) bool {
		return r == '#' || unicode.IsSpace(r)
	})
	heading = strings.TrimSpace(heading)
	return level, heading, heading != ""
}

func definitionByNegation(line string) bool {
	for _, sentence := range sentenceFragments(line) {
		if sentenceDefinitionByNegation(sentence) {
			return true
		}
	}
	return false
}

func sentenceFragments(line string) []string {
	var fragments []string
	rest := strings.TrimSpace(line)
	for rest != "" {
		head, tail, ok := cutSentence(rest)
		if !ok {
			fragments = append(fragments, rest)
			break
		}
		fragments = append(fragments, head)
		rest = strings.TrimSpace(tail)
	}
	return fragments
}

func sentenceDefinitionByNegation(sentence string) bool {
	sentence = strings.TrimSpace(sentence)
	if sentence == "" {
		return false
	}
	if !negationCopulaPattern.MatchString(sentence) {
		return false
	}
	if positivePredicateAfterNegation(sentence) {
		return false
	}
	if positivePredicateBeforeSemicolon(sentence) {
		return false
	}
	return true
}

func positivePredicateAfterNegation(sentence string) bool {
	lower := strings.ToLower(sentence)
	idx := strings.Index(lower, " not ")
	if idx == -1 {
		return false
	}
	tail := strings.TrimSpace(sentence[idx+len(" not "):])
	return positiveCopulaPattern.MatchString(tail)
}

func positivePredicateBeforeSemicolon(sentence string) bool {
	idx := strings.IndexAny(sentence, ";:")
	if idx == -1 {
		return false
	}
	head := strings.TrimSpace(sentence[:idx])
	tail := strings.TrimSpace(sentence[idx+1:])
	if !positiveCopulaPattern.MatchString(head) {
		return false
	}
	return negationCopulaPattern.MatchString(tail)
}

func stripInlineCode(line string) string {
	return inlineCodePattern.ReplaceAllString(line, "")
}

func isFenceLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func fenceDelimiter(trimmed string) string {
	if strings.HasPrefix(trimmed, "```") {
		return "```"
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return "~~~"
	}
	return ""
}

func repeatedOpeningKey(line string) string {
	first, rest, ok := cutSentence(line)
	if !ok {
		return ""
	}
	second, _, ok := cutSentence(rest)
	if !ok {
		return ""
	}
	first = normalizeSentence(first)
	second = normalizeSentence(second)
	if first == "" || first != second {
		return ""
	}
	return first
}

func cutSentence(s string) (head, tail string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			return strings.TrimSpace(s[:i+1]), strings.TrimSpace(s[i+1:]), true
		}
	}
	return "", "", false
}

func normalizeSentence(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func containsWord(haystack, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	pattern := `(?i)(^|[^[:alnum:]_])` + regexp.QuoteMeta(needle) + `([^[:alnum:]_]|$)`
	return regexp.MustCompile(pattern).FindStringIndex(haystack) != nil
}
