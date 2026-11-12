package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

const VectorSize = 64

type Vector [VectorSize]float64

type Features struct {
	Accuracy         float64 `json:"accuracy"`
	ChecksPassed     int     `json:"checks_passed"`
	ChecksTotal      int     `json:"checks_total"`
	Refused          bool    `json:"refused"`
	RefusalHash      string  `json:"refusal_hash,omitempty"`
	ResponseHash     string  `json:"response_hash,omitempty"`
	JSONValid        bool    `json:"json_valid"`
	StructuralScore  float64 `json:"structural_score"`
	WordCount        int     `json:"word_count"`
	ResponseLength   int     `json:"response_length"`
	LatencyMS        int64   `json:"latency_ms"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	FinishReason     string  `json:"finish_reason"`
	Truncated        bool    `json:"truncated"`
	SentenceCount    int     `json:"sentence_count"`
	AverageSentence  float64 `json:"average_sentence_length"`
	BulletCount      int     `json:"bullet_count"`
	Markdown         bool    `json:"markdown"`
	PunctuationRatio float64 `json:"punctuation_ratio"`
	Vector           Vector  `json:"vector"`
}

func Extract(result canary.Result, latencyMS int64) Features {
	content := ""
	finishReason := ""
	if len(result.Response.Choices) > 0 {
		content = result.Response.Choices[0].Message.Content
		finishReason = result.Response.Choices[0].FinishReason
	}
	passed := 0
	for _, check := range result.Checks {
		if check.Passed {
			passed++
		}
	}
	f := Features{
		Accuracy:     passedRatio(passed, len(result.Checks)),
		ChecksPassed: passed, ChecksTotal: len(result.Checks),
		Refused: refusal(content), JSONValid: json.Valid([]byte(strings.TrimSpace(content))),
		WordCount: len(strings.Fields(content)), ResponseLength: len([]rune(content)),
		LatencyMS: latencyMS, InputTokens: result.Response.Usage.PromptTokens,
		OutputTokens: result.Response.Usage.CompletionTokens, FinishReason: finishReason,
		Truncated: finishReason == "length", SentenceCount: sentenceCount(content),
		BulletCount: bulletCount(content), Markdown: strings.Contains(content, "```") ||
			strings.Contains(content, "|") || strings.Contains(content, "# "),
		PunctuationRatio: punctuationRatio(content),
	}
	f.ResponseHash = contentHash(content)
	f.AverageSentence = float64(f.WordCount) / float64(maxInt(f.SentenceCount, 1))
	if f.Refused {
		f.RefusalHash = refusalHash(content)
	}
	f.StructuralScore = structuralScore(f)
	f.Vector = StyleVector(content)
	return f
}

func Marshal(f Features) (string, error) {
	data, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func StyleVector(content string) Vector {
	var vector Vector
	normalized := strings.ToLower(strings.Join(strings.Fields(content), " "))
	// Hashing keeps the monitor dependency-free and prompt-free, but collisions
	// are expected in this compact vector and can make unrelated styles appear
	// closer than they are. It is a comparative signal, not semantic analysis.
	for i := 0; i+2 < len(normalized); i++ {
		if unicode.IsSpace(rune(normalized[i])) {
			continue
		}
		addHash(&vector, normalized[i:i+3])
	}
	for _, word := range strings.Fields(normalized) {
		addHash(&vector, "word:"+word)
	}
	var norm float64
	for _, value := range vector {
		norm += value * value
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}
	return vector
}

func CosineDistance(a, b Vector) float64 {
	var dot, aNorm, bNorm float64
	for i := range a {
		dot += a[i] * b[i]
		aNorm += a[i] * a[i]
		bNorm += b[i] * b[i]
	}
	if aNorm == 0 || bNorm == 0 {
		return 1
	}
	return 1 - dot/(math.Sqrt(aNorm)*math.Sqrt(bNorm))
}

func addHash(vector *Vector, value string) {
	hash := sha256.Sum256([]byte(value))
	index := int(hash[0]) % VectorSize
	if hash[1]&1 == 0 {
		vector[index]++
	} else {
		vector[index]--
	}
}

func refusal(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{"i can't", "i cannot", "i'm unable", "i won't", "can't help"} {
		if strings.HasPrefix(lower, phrase) {
			return true
		}
	}
	return false
}

func refusalHash(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	hash := sha256.Sum256([]byte(lower[:minInt(len(lower), 200)]))
	return hex.EncodeToString(hash[:])[:16]
}

func contentHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])[:16]
}

func sentenceCount(value string) int {
	count := len(regexp.MustCompile(`[.!?]+`).FindAllString(value, -1))
	if count == 0 && strings.TrimSpace(value) != "" {
		return 1
	}
	return count
}

func bulletCount(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			count++
		}
	}
	return count
}

func punctuationRatio(value string) float64 {
	runes := []rune(value)
	if len(runes) == 0 {
		return 0
	}
	count := 0
	for _, r := range runes {
		if unicode.IsPunct(r) {
			count++
		}
	}
	return float64(count) / float64(len(runes))
}

func structuralScore(f Features) float64 {
	score := 0.0
	if f.JSONValid {
		score += 0.25
	}
	if f.WordCount > 0 {
		score += 0.25
	}
	if f.SentenceCount > 0 {
		score += 0.25
	}
	if f.FinishReason == "stop" {
		score += 0.25
	}
	return score
}

func passedRatio(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) / float64(total)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
