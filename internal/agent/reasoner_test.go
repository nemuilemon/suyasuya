package agent

import (
	"strings"
	"testing"
)

func TestAgentDecision(t *testing.T) {
	for _, decision := range []Decision{
		{Send: true, Body: "最近は時刻がそろってきたね。今日はここまでにしよう。", Reason: "継続を肯定しながら就寝を促す"},
		{Send: false, Reason: "直近に同じ通知を送っているため見送る"},
	} {
		if err := validateDecision(decision); err != nil {
			t.Fatal(err)
		}
	}
	for _, decision := range []Decision{
		{Send: true, Reason: "empty"}, {Send: false}, {Send: true, Body: strings.Repeat("長", 1501), Reason: "too long"},
	} {
		if validateDecision(decision) == nil {
			t.Fatal("invalid output accepted")
		}
	}
}
