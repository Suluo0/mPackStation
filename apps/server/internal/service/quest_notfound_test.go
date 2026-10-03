package service

import (
	"context"
	"errors"
	"testing"
)

// O4 守护用例：GET /quests 对「包存在但还没写任务书」不能再报 pack_not_found。
//
// 回归的根因是 store.ErrNotFound 一路抛到 HTTP 层，被兜底翻译成「pack not found」，
// 于是任务页对一个真实存在的包说「找不到整合包」。
func TestGetQuestSeparatesMissingBookFromMissingPack(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	var de *DomainError
	if _, err := app.GetQuest(ctx, packID); !errors.As(err, &de) {
		t.Fatalf("missing quest book must be a DomainError, got %v", err)
	} else if de.Status != 404 || de.Code != "quest_book_not_found" {
		t.Errorf("quest book: got %d %s, want 404 quest_book_not_found", de.Status, de.Code)
	}

	if _, err := app.GetQuest(ctx, "pack-does-not-exist"); !errors.As(err, &de) {
		t.Fatalf("missing pack must be a DomainError, got %v", err)
	} else if de.Code != "pack_not_found" {
		t.Errorf("missing pack: got %s, want pack_not_found", de.Code)
	}

	// 预览/校验/应用都经由 GetQuest，语义必须一起收敛，不能各自回到 pack_not_found。
	for name, call := range map[string]func() error{
		"preview":  func() error { _, e := app.QuestPreview(ctx, packID); return e },
		"validate": func() error { _, e := app.ValidateQuest(ctx, packID, "req"); return e },
		"apply":    func() error { _, e := app.ApplyQuest(ctx, packID, "req"); return e },
	} {
		if err := call(); !errors.As(err, &de) || de.Code != "quest_book_not_found" {
			t.Errorf("%s: got %v, want quest_book_not_found", name, err)
		}
	}
}

// 回滚到不存在的修订应说明是修订找不到，而不是包找不到。
func TestRollbackQuestNamesMissingRevision(t *testing.T) {
	db, app, packID, _ := newP7Fixture(t)
	t.Cleanup(func() { _ = db.Close() })

	_, err := app.RollbackQuest(context.Background(), packID, "quest-revision-nope", "req")
	var de *DomainError
	if !errors.As(err, &de) || de.Code != "quest_revision_not_found" {
		t.Fatalf("got %v, want quest_revision_not_found", err)
	}
}
