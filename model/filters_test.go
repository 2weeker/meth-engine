package model

import (
	"reflect"
	"strings"
	"testing"
)

func compiled(t *testing.T, fs ...Filter) []compiledFilter {
	t.Helper()
	out, broken := compileFilters(fs)
	if len(broken) > 0 {
		t.Fatalf("patterns did not compile: %v", broken)
	}
	return out
}

func TestEvaluateFilters(t *testing.T) {
	fs := compiled(t,
		Filter{ID: 1, Regex: `(?i)darn`, Action: FilterReplace, Replacement: "d***"},
		Filter{ID: 2, Regex: `heck`, Action: FilterReplace, Replacement: ""},
		Filter{ID: 3, Regex: `(?i)buy pills`, Action: FilterReject},
		Filter{ID: 4, Regex: `casino\.example`, Action: FilterBan, BanDays: 7, Note: "casino spam"},
		Filter{ID: 5, Regex: `(?i)worst thing`, Action: FilterBan, BanDays: 0},
		Filter{ID: 6, Regex: `\$money`, Action: FilterReplace, Replacement: "$1 literal"},
	)
	for name, c := range map[string]struct {
		msg     string
		action  FilterAction
		out     string
		matched []int64
		days    int
		by      int64
	}{
		"nothing matches":         {msg: "hello there", action: "", out: "hello there"},
		"replace":                 {msg: "well DARN it", action: FilterReplace, out: "well d*** it", matched: []int64{1}},
		"replace with nothing":    {msg: "what the heck", action: FilterReplace, out: "what the ", matched: []int64{2}},
		"two replacements":        {msg: "darn heck darn", action: FilterReplace, out: "d***  d***", matched: []int64{1, 2}},
		"replacement is literal":  {msg: "send $money", action: FilterReplace, out: "send $1 literal", matched: []int64{6}},
		"reject":                  {msg: "Buy Pills now", action: FilterReject, out: "Buy Pills now", matched: []int64{3}, by: 3},
		"reject beats replace":    {msg: "darn, buy pills", action: FilterReject, out: "darn, buy pills", matched: []int64{1, 3}, by: 3},
		"ban":                     {msg: "visit casino.example", action: FilterBan, out: "visit casino.example", matched: []int64{4}, days: 7, by: 4},
		"ban beats reject":        {msg: "buy pills at casino.example", action: FilterBan, out: "buy pills at casino.example", matched: []int64{3, 4}, days: 7, by: 4},
		"permanent beats 7 days":  {msg: "casino.example is the worst thing", action: FilterBan, out: "casino.example is the worst thing", matched: []int64{4, 5}, days: 0, by: 5},
		"dot does not match text": {msg: "casinoXexample", action: "", out: "casinoXexample"},
	} {
		got := evaluateFilters(fs, c.msg)
		if got.Action != c.action || got.Message != c.out || !reflect.DeepEqual(got.Matched, c.matched) || got.BanDays != c.days || got.By.ID != c.by {
			t.Errorf("%s: got action=%q message=%q matched=%v days=%d by=%d", name, got.Action, got.Message, got.Matched, got.BanDays, got.By.ID)
		}
	}
}

func TestEvaluateFiltersAcrossLines(t *testing.T) {
	fs := compiled(t, Filter{ID: 1, Regex: `start.*end`, Action: FilterReject})
	if got := evaluateFilters(fs, "start\nmiddle\nend"); got.Action != FilterReject {
		t.Errorf("a pattern must match across line breaks: %+v", got)
	}
}

func TestCompileFiltersSkipsBroken(t *testing.T) {
	ok, broken := compileFilters([]Filter{
		{ID: 1, Regex: `fine`, Action: FilterReject},
		{ID: 2, Regex: `(unclosed`, Action: FilterBan},
	})
	if len(ok) != 1 || ok[0].ID != 1 || !reflect.DeepEqual(broken, []int64{2}) {
		t.Errorf("a broken pattern is skipped, not fatal: ok=%v broken=%v", ok, broken)
	}
}

func TestCheckNewFilter(t *testing.T) {
	good := []NewFilter{
		{Regex: `spam`, Action: FilterReject},
		{Regex: `spam`, Action: FilterReject, Note: "why"},
		{Regex: `darn`, Action: FilterReplace, Replacement: "d***"},
		{Regex: `darn`, Action: FilterReplace, Replacement: ""},
		{Regex: `bot`, Action: FilterBan, BanDays: 0},
		{Regex: `bot`, Action: FilterBan, BanDays: 30},
	}
	for _, f := range good {
		if err := checkNewFilter(f); err != nil {
			t.Errorf("%+v should be accepted: %v", f, err)
		}
	}
	bad := map[string]NewFilter{
		"empty pattern":           {Regex: ``, Action: FilterReject},
		"blank pattern":           {Regex: `   `, Action: FilterReject},
		"matches every post":      {Regex: `a*`, Action: FilterReject},
		"also matches everything": {Regex: `.*`, Action: FilterBan},
		"does not compile":        {Regex: `(unclosed`, Action: FilterReject},
		"too long":                {Regex: strings.Repeat("a", MaxRegexLength+1), Action: FilterReject},
		"unknown action":          {Regex: `x`, Action: "delete"},
		"no action":               {Regex: `x`},
		"negative ban":            {Regex: `x`, Action: FilterBan, BanDays: -1},
		"absurd ban":              {Regex: `x`, Action: FilterBan, BanDays: 100000},
		"long replacement":        {Regex: `x`, Action: FilterReplace, Replacement: strings.Repeat("y", 300)},
		"long note":               {Regex: `x`, Action: FilterReject, Note: strings.Repeat("n", 300)},
	}
	for name, f := range bad {
		if err := checkNewFilter(f); err == nil {
			t.Errorf("%s: %+v should be refused", name, f)
		}
	}
}

func TestFilterLabels(t *testing.T) {
	for want, f := range map[string]Filter{
		"reject":               {Action: FilterReject},
		"replace with “d***”":  {Action: FilterReplace, Replacement: "d***"},
		"replace with nothing": {Action: FilterReplace},
		"ban for 1 day":        {Action: FilterBan, BanDays: 1},
		"ban for 7 days":       {Action: FilterBan, BanDays: 7},
		"ban permanently":      {Action: FilterBan},
	} {
		if got := f.ActionLabel(); got != want {
			t.Errorf("ActionLabel() = %q, want %q", got, want)
		}
	}
}
