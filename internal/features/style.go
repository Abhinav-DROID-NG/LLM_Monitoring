package features

import (
	"math"
	"strings"
	"unicode"
)

// StyleVectorV1 returns lexical and structural features in the documented
// order. It is intentionally provider-independent and uses no embeddings.
func StyleVectorV1(text string) [9]float64 {
	words := strings.Fields(text)
	sentences := splitSentences(text)
	paragraphs := nonEmptyParagraphs(text)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var punctuation, uppercase, letters, contractions, bullets float64
	unique := make(map[string]struct{})
	for _, word := range words {
		unique[strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}"))] = struct{}{}
		if strings.ContainsAny(word, "'’") {
			contractions++
		}
	}
	for _, r := range text {
		if unicode.IsPunct(r) {
			punctuation++
		}
		if unicode.IsUpper(r) {
			uppercase++
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "• ") {
			bullets++
		}
	}
	meanSentence := 0.0
	for _, sentence := range sentences {
		meanSentence += float64(len(strings.Fields(sentence)))
	}
	if len(sentences) > 0 {
		meanSentence /= float64(len(sentences))
	}
	variance := 0.0
	for _, sentence := range sentences {
		delta := float64(len(strings.Fields(sentence))) - meanSentence
		variance += delta * delta
	}
	if len(sentences) > 0 {
		variance /= float64(len(sentences))
	}
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	wordCount := float64(len(words))
	return [9]float64{
		math.Log1p(wordCount),
		meanSentence,
		math.Sqrt(variance),
		safeDivide(float64(len(unique)), wordCount),
		safeDivide(punctuation, float64(len([]rune(text)))),
		safeDivide(uppercase, letters),
		float64(len(paragraphs)),
		safeDivide(bullets, float64(nonEmptyLines)),
		safeDivide(contractions, wordCount),
	}
}

func Distance(a, b [9]float64) float64 {
	var sum float64
	for i := range a {
		delta := a[i] - b[i]
		sum += delta * delta
	}
	return math.Sqrt(sum)
}

func splitSentences(text string) []string {
	var result []string
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	}) {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}
	return result
}

func nonEmptyParagraphs(text string) []string {
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}
	return result
}

func safeDivide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
