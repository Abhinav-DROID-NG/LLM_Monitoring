package canary

import "testing"

func TestEvaluateCheckTypes(t *testing.T) {
	tests := []struct {
		name string
		c    Check
		out  string
	}{
		{"output_equals", Check{Type: "output_equals", Value: "hello"}, "hello"},
		{"regex_full", Check{Type: "regex_full", Value: `hello [0-9]+`}, "hello 7"},
		{"contains_any", Check{Type: "contains_any", Values: []string{"cat", "dog"}}, "A DOG"},
		{"contains_none", Check{Type: "contains_none", Values: []string{"cat", "dog"}}, "bird"},
		{"number_equals", Check{Type: "number_equals", Value: "3.5"}, "3.5"},
		{"number_in_set", Check{Type: "number_in_set", Values: []string{"2", "3"}}, "3"},
		{"json_fields", Check{Type: "json_fields", Values: []string{"a", "b"}}, `{"a":1,"b":true}`},
		{"json_types", Check{Type: "json_types", Values: []string{"a:number", "b:boolean"}}, `{"a":1,"b":true}`},
		{"json_array", Check{Type: "json_array", Values: []string{"one", "two"}}, `["one","two"]`},
		{"csv_shape", Check{Type: "csv_shape", Value: "2x2"}, "a,b\nc,d"},
		{"outline_shape", Check{Type: "outline_shape", Values: []string{"2"}}, "# A\n## B"},
		{"markdown_table", Check{Type: "markdown_table"}, "|a|\n|-|\n|b|"},
		{"table_cells", Check{Type: "table_cells", Values: []string{"a", "b"}}, "|a|b|"},
		{"bullet_count", Check{Type: "bullet_count", Value: "2"}, "- one\n- two"},
		{"bullet_words_max", Check{Type: "bullet_words_max", Value: "2"}, "- one\n- two"},
		{"line_count", Check{Type: "line_count", Value: "2"}, "one\ntwo"},
		{"word_count_max", Check{Type: "word_count_max", Value: "2"}, "one two"},
		{"word_count_between", Check{Type: "word_count_between", Values: []string{"2", "3"}}, "one two"},
		{"paragraph_count", Check{Type: "paragraph_count", Value: "2"}, "one\n\ntwo"},
		{"paragraph_count_max", Check{Type: "paragraph_count_max", Value: "2"}, "one\n\ntwo"},
		{"sentence_count", Check{Type: "sentence_count", Value: "2"}, "One. Two!"},
		{"sentences_per_paragraph", Check{Type: "sentences_per_paragraph", Value: "2"}, "One. Two!"},
		{"starts_with", Check{Type: "starts_with", Value: "Hello"}, "Hello world"},
		{"ends_with", Check{Type: "ends_with", Value: "world"}, "Hello world"},
		{"first_line_equals", Check{Type: "first_line_equals", Value: "Title"}, "Title\nbody"},
		{"no_code_fence", Check{Type: "no_code_fence"}, "plain text"},
		{"all_lowercase", Check{Type: "all_lowercase"}, "plain text"},
		{"language_is", Check{Type: "language_is", Value: "english"}, "This is a simple sentence."},
		{"final_field", Check{Type: "final_field", Value: "Answer"}, "Work\nAnswer: yes"},
		{"final_field_json", Check{Type: "final_field_json", Value: "answer"}, `{"answer":true}`},
		{"day_list_shape", Check{Type: "day_list_shape"}, "Monday Tuesday Wednesday Thursday Friday Saturday Sunday"},
		{"summary_words_max", Check{Type: "summary_words_max", Value: "3"}, "one two three"},
		{"finish_reason", Check{Type: "finish_reason", Value: "stop"}, ""},
		{"contains", Check{Type: "contains", Value: "hello"}, "hello"},
		{"contains_all", Check{Type: "contains_all", Values: []string{"a", "b"}}, "a b"},
		{"json_valid", Check{Type: "json_valid"}, `{"ok":true}`},
		{"not_refused", Check{Type: "not_refused"}, "Here is the answer."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateCheckWithFinishReason(tt.c, tt.out, map[bool]string{true: "stop", false: ""}[tt.name == "finish_reason"])
			if !got {
				t.Fatalf("check did not pass: %+v on %q", tt.c, tt.out)
			}
		})
	}
}

func TestEvaluateCheckFailures(t *testing.T) {
	tests := []struct {
		c   Check
		out string
	}{
		{Check{Type: "output_equals", Value: "yes"}, "no"},
		{Check{Type: "contains_none", Values: []string{"bad"}}, "bad"},
		{Check{Type: "json_valid"}, "not json"},
		{Check{Type: "bullet_count", Value: "2"}, "- one"},
		{Check{Type: "not_refused"}, "I can't help with that."},
	}
	for _, tt := range tests {
		if EvaluateCheck(tt.c, tt.out) {
			t.Errorf("check unexpectedly passed: %+v on %q", tt.c, tt.out)
		}
	}
}

func TestValidateCheckRejectsUnknown(t *testing.T) {
	if err := ValidateCheck(Check{Type: "made_up"}); err == nil {
		t.Fatal("unknown check type was accepted")
	}
	if err := ValidateCheck(Check{Type: "regex_full", Value: "["}); err == nil {
		t.Fatal("invalid regex was accepted")
	}
}
