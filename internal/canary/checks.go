package canary

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// These names describe groups of checks in library files. They are accepted by
// the schema validator, but are not executable checks themselves.
var aggregateCheckTypes = map[string]bool{
	"all": true, "all_of": true, "any": true, "any_of": true,
	"aggregate": true, "weighted": true, "rate": true, "at_least": true,
	"exactly": true, "score": true, "all_checks": true, "any_checks": true,
	"identical_across_runs": true, "mean_edit_distance": true,
	"input_tokens_stable": true, "output_tokens_stable": true, "latency": true,
	"output_length_stats": true, "style_feature_distance": true,
}

var checkTypes = map[string]bool{
	"output_equals": true, "regex_full": true, "contains_any": true,
	"contains_none": true, "number_equals": true, "number_in_set": true,
	"json_fields": true, "json_types": true, "json_array": true,
	"csv_shape": true, "outline_shape": true, "markdown_table": true,
	"table_cells": true, "bullet_count": true, "bullet_words_max": true,
	"line_count": true, "word_count_max": true, "word_count_between": true,
	"paragraph_count": true, "paragraph_count_max": true, "sentence_count": true,
	"sentences_per_paragraph": true, "starts_with": true, "ends_with": true,
	"first_line_equals": true, "no_code_fence": true, "all_lowercase": true,
	"language_is": true, "final_field": true, "final_field_json": true,
	"day_list_shape": true, "summary_words_max": true, "finish_reason": true,
	// Legacy names.
	"contains": true, "contains_all": true, "json_valid": true, "not_refused": true,
}

func ValidateCheck(c Check) error {
	if strings.TrimSpace(c.Type) == "" {
		return errors.New("check type is empty")
	}
	if !checkTypes[c.Type] && !aggregateCheckTypes[c.Type] {
		return fmt.Errorf("unknown check type %q", c.Type)
	}
	if c.Type == "regex_full" {
		if _, err := regexp.Compile(c.ValueString()); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	}
	return nil
}

func ValidateChecks(checks []Check) error {
	for i, check := range checks {
		if err := ValidateCheck(check); err != nil {
			return fmt.Errorf("check %d: %w", i, err)
		}
	}
	return nil
}

func KnownCheckType(name string) bool { return checkTypes[name] || aggregateCheckTypes[name] }

type Result struct {
	Passed      bool
	Unavailable bool
	Detail      string
}

type ResponseView struct {
	Text         string
	FinishReason string
	InputTokens  *int
	OutputTokens *int
	Error        string
}

type PerResponseChecker func(task Task, check Check, response ResponseView) Result

var Registry = buildRegistry()

func buildRegistry() map[string]PerResponseChecker {
	registry := make(map[string]PerResponseChecker, len(checkTypes)+len(aggregateCheckTypes))
	for name := range checkTypes {
		registry[name] = func(task Task, check Check, response ResponseView) Result {
			if response.Error != "" {
				return Result{Detail: response.Error}
			}
			return Result{Passed: EvaluateCheckWithFinishReason(check, response.Text, response.FinishReason)}
		}
	}
	for name := range aggregateCheckTypes {
		registry[name] = func(Task, Check, ResponseView) Result {
			return Result{Unavailable: true, Detail: "aggregate"}
		}
	}
	registry["finish_reason"] = func(_ Task, check Check, response ResponseView) Result {
		if response.FinishReason == "" || response.FinishReason == "unknown" {
			return Result{Unavailable: true, Detail: "finish_reason unavailable"}
		}
		return Result{Passed: response.FinishReason == check.ValueString()}
	}
	registry["not_refused"] = func(_ Task, _ Check, response ResponseView) Result {
		class, providerFilter := ClassifyRefusal(response.Text, response.FinishReason)
		if providerFilter {
			return Result{Passed: false, Detail: "provider content filter"}
		}
		return Result{Passed: class == NotRefusal, Detail: refusalClassName(class)}
	}
	return registry
}

func CheckResponse(task Task, check Check, response ResponseView) Result {
	checker, ok := Registry[check.Type]
	if !ok {
		return Result{Detail: "unregistered check type"}
	}
	return checker(task, check, response)
}

// EvaluateCheck evaluates a textual response. Checks needing provider metadata
// (finish_reason) can be evaluated with EvaluateCheckWithFinishReason.
func EvaluateCheck(c Check, output string) bool {
	return EvaluateCheckWithFinishReason(c, output, "")
}

func Evaluate(c Check, output string) bool { return EvaluateCheck(c, output) }

func EvaluateCheckWithFinishReason(c Check, output, finishReason string) bool {
	if aggregateCheckTypes[c.Type] {
		return false
	}
	value := strings.TrimSpace(c.ValueString())
	values := c.ValuesList()
	if len(values) == 0 && value != "" {
		values = []string{value}
	}
	switch c.Type {
	case "output_equals":
		return strings.TrimSpace(output) == value
	case "regex_full":
		r, err := regexp.Compile(value)
		return err == nil && r.MatchString(strings.TrimSpace(output))
	case "contains", "contains_any":
		for _, v := range values {
			if strings.Contains(strings.ToLower(output), strings.ToLower(v)) {
				return true
			}
		}
		return false
	case "contains_all":
		for _, v := range values {
			if !strings.Contains(strings.ToLower(output), strings.ToLower(v)) {
				return false
			}
		}
		return true
	case "contains_none":
		for _, v := range values {
			if strings.Contains(strings.ToLower(output), strings.ToLower(v)) {
				return false
			}
		}
		return true
	case "number_equals":
		a, e1 := firstNumber(output)
		b, e2 := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64)
		return e1 && e2 == nil && math.Abs(a-b) <= 1e-9
	case "number_in_set":
		a, ok := firstNumber(output)
		if !ok {
			return false
		}
		for _, v := range values {
			b, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", ""), 64)
			if err == nil && math.Abs(a-b) <= 1e-9 {
				return true
			}
		}
		return false
	case "json_valid":
		var v any
		return json.Unmarshal([]byte(output), &v) == nil
	case "json_fields":
		var obj map[string]any
		if json.Unmarshal([]byte(output), &obj) != nil {
			return false
		}
		for _, field := range values {
			if _, ok := obj[field]; !ok {
				return false
			}
		}
		return true
	case "json_types":
		var obj map[string]any
		if json.Unmarshal([]byte(output), &obj) != nil {
			return false
		}
		for _, spec := range values {
			parts := strings.SplitN(spec, ":", 2)
			if len(parts) != 2 || jsonType(obj[parts[0]]) != strings.ToLower(parts[1]) {
				return false
			}
		}
		return true
	case "json_array":
		var arr []any
		if json.Unmarshal([]byte(output), &arr) != nil {
			return false
		}
		if len(values) == 0 {
			return true
		}
		for _, want := range values {
			found := false
			for _, item := range arr {
				if fmt.Sprint(item) == want {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	case "csv_shape":
		rows, err := csvRows(output)
		if err != nil {
			return false
		}
		r, col, ok := shape(value, values)
		if !ok {
			return false
		}
		return len(rows) == r && (col < 0 || allColumns(rows, col))
	case "outline_shape":
		return outlineShape(output, values)
	case "markdown_table":
		return markdownTable(output, value, values)
	case "table_cells":
		return tableCells(output, values)
	case "bullet_count":
		return len(bullets(output)) == integer(value, values, -1)
	case "bullet_words_max":
		max := integer(value, values, -1)
		if max < 0 {
			return false
		}
		for _, b := range bullets(output) {
			if wordCount(b) > max {
				return false
			}
		}
		return true
	case "line_count":
		return lineCount(output) == integer(value, values, -1)
	case "word_count_max", "summary_words_max":
		return wordCount(output) <= integer(value, values, -1)
	case "word_count_between":
		if len(values) < 2 {
			values = strings.Fields(value)
		}
		if len(values) < 2 {
			return false
		}
		return wordCount(output) >= integer(values[0], nil, -1) && wordCount(output) <= integer(values[1], nil, -1)
	case "paragraph_count":
		return len(paragraphs(output)) == integer(value, values, -1)
	case "paragraph_count_max":
		return len(paragraphs(output)) <= integer(value, values, -1)
	case "sentence_count":
		return sentenceCount(output) == integer(value, values, -1)
	case "sentences_per_paragraph":
		want := integer(value, values, -1)
		if want < 0 {
			return false
		}
		for _, p := range paragraphs(output) {
			if sentenceCount(p) != want {
				return false
			}
		}
		return true
	case "starts_with":
		return strings.HasPrefix(output, value)
	case "ends_with":
		return strings.HasSuffix(strings.TrimSpace(output), value)
	case "first_line_equals":
		return strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")[0] == value
	case "no_code_fence":
		return !strings.Contains(output, "```")
	case "all_lowercase":
		return strings.ToLower(output) == output
	case "language_is":
		return language(output, value)
	case "final_field", "final_field_json":
		return finalField(output, value, c.Type == "final_field_json")
	case "day_list_shape":
		return dayList(output, values)
	case "not_refused":
		l := strings.ToLower(output)
		for _, p := range []string{"i can't", "i cannot", "i'm unable", "i won't", "can't help"} {
			if strings.Contains(l, p) {
				return false
			}
		}
		return true
	case "finish_reason":
		return finishReason != "" && finishReason == value
	}
	return false
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}
func integer(v string, vs []string, d int) int {
	if v == "" && len(vs) > 0 {
		v = vs[0]
	}
	n, e := strconv.Atoi(strings.TrimSpace(v))
	if e != nil {
		return d
	}
	return n
}
func wordCount(s string) int { return len(strings.Fields(s)) }

var numberPattern = regexp.MustCompile(`-?\d+(?:,\d{3})*(?:\.\d+)?`)

func firstNumber(text string) (float64, bool) {
	if answer := regexp.MustCompile(`(?i)\bANSWER\s*:\s*`).FindStringIndex(text); answer != nil {
		text = text[answer[1]:]
	}
	match := numberPattern.FindString(text)
	if match == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(match, ",", ""), 64)
	return value, err == nil
}
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n"))
}
func paragraphs(s string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(s), -1) {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}
func sentenceCount(s string) int {
	n := 0
	for _, r := range s {
		if r == '.' || r == '!' || r == '?' {
			n++
		}
	}
	return n
}
func bullets(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if len(l) > 1 && ((l[0] == '-' || l[0] == '*' || l[0] == '+') && unicode.IsSpace(rune(l[1])) || (l[0] >= '0' && l[0] <= '9' && strings.Contains(l, ". "))) {
			out = append(out, l)
		}
	}
	return out
}
func shape(v string, vs []string) (int, int, bool) {
	if len(vs) > 0 {
		v = vs[0]
	}
	p := strings.FieldsFunc(v, func(r rune) bool { return r == 'x' || r == ',' || r == ':' })
	if len(p) < 1 {
		return 0, 0, false
	}
	r, e := strconv.Atoi(p[0])
	if e != nil {
		return 0, 0, false
	}
	c := -1
	if len(p) > 1 {
		c, e = strconv.Atoi(p[1])
		if e != nil {
			return 0, 0, false
		}
	}
	return r, c, true
}
func csvRows(s string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.TrimLeadingSpace = true
	return r.ReadAll()
}
func allColumns(rows [][]string, n int) bool {
	for _, r := range rows {
		if len(r) != n {
			return false
		}
	}
	return true
}
func outlineShape(s string, vs []string) bool {
	heads := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			heads++
		}
	}
	want := integer("", vs, -1)
	return want >= 0 && heads == want
}
func markdownTable(s, v string, vs []string) bool {
	rows := strings.Split(strings.TrimSpace(s), "\n")
	if len(rows) < 2 || !strings.Contains(rows[0], "|") || !strings.Contains(rows[1], "-") {
		return false
	}
	if v != "" || len(vs) > 0 {
		return len(rows) == integer(v, vs, -1)
	}
	return true
}
func tableCells(s string, vs []string) bool {
	for _, v := range vs {
		if !strings.Contains(s, v) {
			return false
		}
	}
	return true
}
func language(s, want string) bool {
	want = strings.ToLower(want)
	if want == "en" || want == "english" {
		return strings.ContainsAny(strings.ToLower(s), " the a an and is are of to ")
	}
	return true
}
func finalField(s, field string, jsonMode bool) bool {
	if jsonMode {
		var o map[string]any
		if json.Unmarshal([]byte(s), &o) != nil {
			return false
		}
		_, ok := o[field]
		return ok
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return len(lines) > 0 && strings.Contains(lines[len(lines)-1], field)
}
func dayList(s string, vs []string) bool {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	n := 0
	for _, d := range days {
		if strings.Contains(strings.ToLower(s), d) {
			n++
		}
	}
	return n == 7 || (len(vs) > 0 && n == len(vs))
}
