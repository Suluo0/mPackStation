package service

import (
	"testing"

	"mpackstation/internal/provider"
)

func TestExpandModSearchAlias(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"精致存储", "sophisticated-storage"},
		{"应用能源2", "ae2"},
		{"ae2", "ae2"},
		{"aer", "ae2"},     // 编辑距离 1
		{"sop", ""},        // 无别名命中（排序分兜底）
		{"随便什么", ""},
	}
	for _, c := range cases {
		if got := expandModSearchAlias(c.in); got != c.want {
			t.Fatalf("expandModSearchAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestModSearchScoreOrdering(t *testing.T) {
	q := "ae2"
	mk := func(slug, name string) provider.Project {
		return provider.Project{Slug: slug, Name: name, Downloads: 1000}
	}
	main := mk("ae2", "Applied Energistics 2")             // slug 精确 → 100
	addon := mk("ae2-emi-crafting", "AE2 EMI Crafting Integration") // 名称分词前缀 → 80
	if modSearchScore(q, main) <= modSearchScore(q, addon) {
		t.Fatalf("slug-exact must outrank title-prefix addon: %d vs %d", modSearchScore(q, main), modSearchScore(q, addon))
	}
	addon2 := mk("ae2-things", "AE2 Things")
	if modSearchScore(q, addon) <= modSearchScore(q, addon2) && addon.Downloads >= addon2.Downloads {
		// 同分按下载量兜底：都合理，只断言主序
		_ = addon2
	}
	if modSearchScore("", mk("x", "Anything")) != 50 {
		t.Fatalf("empty query default score")
	}
	if got := modSearchScore(q, mk("unrelated", "Something Else")); got != 10 {
		t.Fatalf("no-match score = %d, want 10", got)
	}
}

func TestLevenshtein(t *testing.T) {
	if levenshtein("aer", "ae2") != 1 || levenshtein("abc", "abc") != 0 || levenshtein("abc", "xyz") != 3 {
		t.Fatal("levenshtein broken")
	}
}

func TestFuzzySlugScore(t *testing.T) {
	got := modSearchScore("aer", provider.Project{Slug: "ae2", Name: "Applied Energistics 2", Downloads: 70000000})
	if got <= 80 {
		t.Fatalf("fuzzy slug score = %d, want > 80", got)
	}
}
