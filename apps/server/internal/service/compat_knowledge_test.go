package service

import (
	"testing"

	"mpackstation/internal/store"
)

func TestBaselineCompatKnowledgeParses(t *testing.T) {
	k := baselineCompatKnowledge()
	if len(k.Recommendations) == 0 {
		t.Fatal("embedded baseline must ship curated recommendations")
	}
	for _, r := range k.Recommendations {
		if r.MR == "" && r.CF == "" {
			t.Fatalf("recommendation %q has no platform identity", r.Name)
		}
		if r.Name == "" || r.Reason == "" {
			t.Fatalf("recommendation %+v missing name/reason", r)
		}
	}
}

func TestEndpointMatchUsesBothSides(t *testing.T) {
	m := store.PackModRecord{ProjectID: "mr-1", MirrorProjectID: "cf-1"}
	if !endpointMatch(m, compatEndpoint{MR: "mr-1"}) {
		t.Fatal("primary id must match")
	}
	if !endpointMatch(m, compatEndpoint{CF: "cf-1"}) {
		t.Fatal("mirror id must match")
	}
	if endpointMatch(m, compatEndpoint{MR: "other"}) {
		t.Fatal("unrelated id must not match")
	}
}

func TestScopeOK(t *testing.T) {
	if !scopeOK("1.20.1", "forge", nil, nil) {
		t.Fatal("empty scope must apply everywhere")
	}
	if scopeOK("1.20.1", "forge", []string{"1.21.1"}, nil) {
		t.Fatal("mc scope must filter")
	}
	if !scopeOK("1.21.1", "NeoForge", nil, []string{"neoforge"}) {
		t.Fatal("loader scope must be case-insensitive")
	}
	if scopeOK("1.21.1", "fabric", nil, []string{"forge"}) {
		t.Fatal("loader scope must filter")
	}
}

func TestScanCompatKnowledge(t *testing.T) {
	k := compatKnowledge{KnownIssues: []compatIssue{{
		A: compatEndpoint{MR: "a-mr", CF: "a-cf"}, B: compatEndpoint{MR: "b-mr", CF: "b-cf"},
		Severity: "fatal", Summary: "A 与 B 冲突",
		Fix: &compatFix{Type: "install_mod", Mod: compatEndpoint{MR: "fix-mr", CF: "fix-cf"}, Note: "加装 Fix"},
	}}}
	mods := []store.PackModRecord{
		{ID: "1", ProjectID: "a-mr", MirrorProjectID: "a-cf", Status: "installed"},
		{ID: "2", ProjectID: "b-cf", Status: "installed"}, // 通过 CF 侧命中 B
		{ID: "3", ProjectID: "unrelated", Status: "installed"},
		{ID: "4", ProjectID: "b-mr", Status: "removed"}, // 已移除不算
	}
	hits := scanCompatKnowledge("1.20.1", "forge", mods, k)
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].ModA.ID != "1" || hits[0].ModB.ID != "2" {
		t.Fatalf("hit mods wrong: %+v", hits[0])
	}
	// 只有单边时不命中
	hits = scanCompatKnowledge("1.20.1", "forge", mods[:1], k)
	if len(hits) != 0 {
		t.Fatalf("single side must not hit, got %d", len(hits))
	}
}

func TestFixAlreadyPresent(t *testing.T) {
	fix := &compatFix{Type: "install_mod", Mod: compatEndpoint{MR: "fix-mr", CF: "fix-cf"}}
	mods := []store.PackModRecord{{ID: "1", ProjectID: "x", MirrorProjectID: "fix-cf", Status: "installed"}}
	if !fixAlreadyPresent(mods, fix) {
		t.Fatal("mirror-side presence must count")
	}
	mods[0].Status = "removed"
	if fixAlreadyPresent(mods, fix) {
		t.Fatal("removed mod must not count")
	}
}
