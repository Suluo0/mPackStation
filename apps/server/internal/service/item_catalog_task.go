package service

import (
	"context"
	"encoding/json"
	"fmt"

	"mpackstation/internal/task"
)

type catalogInitPayload struct {
	PackID string `json:"packId"`
	Locale string `json:"locale"`
}

// SubmitCatalogInit queues initialization; explicit submissions also retry transient asset warnings.
func (a *API) SubmitCatalogInit(ctx context.Context, packID, locale string) (*task.Task, error) {
	if a == nil || a.queue == nil {
		return nil, ErrUnavailable
	}
	if _, err := a.repo.GetPack(ctx, packID); err != nil { return nil, err }
	payload, _ := json.Marshal(catalogInitPayload{PackID: packID, Locale: catalogLocale(locale)})
	item, _, err := a.queue.Submit(ctx, task.SubmitRequest{PackID: strPtr(packID), Kind: task.KindCatalogInit, Title: "初始化 Minecraft 内容目录", Payload: payload, MaxAttempts: 3})
	return item, err
}

// HandleCatalogInitTask imports version resources and atomically publishes the catalog.
func (a *API) HandleCatalogInitTask(ctx context.Context, execution *task.Execution) (runErr error) {
	var payload catalogInitPayload
	if err := json.Unmarshal(execution.Task.Payload, &payload); err != nil {
		return &task.TaskError{Code: "invalid_payload", Message: "catalog task payload is invalid"}
	}
	if err := a.repo.SetCatalogBuildState(ctx, payload.PackID, "running", ""); err != nil {
		return &task.TaskError{Code: "db_error", Message: "failed to start catalog initialization", Retryable: true}
	}
	defer func() {
		if runErr != nil {
			_ = a.repo.SetCatalogBuildState(context.Background(), payload.PackID, "failed", runErr.Error())
		}
	}()
	_ = execution.Progress(ctx, 10, "读取 Minecraft 版本资源")
	catalog, err := a.RebuildItemCatalog(ctx, payload.PackID, payload.Locale, "task:"+execution.Task.ID)
	if err != nil {
		return &task.TaskError{Code: "catalog_init_failed", Message: err.Error(), Retryable: true}
	}
	_ = execution.Progress(ctx, 100, "内容目录初始化完成")
	execution.Succeed(ctx, fmt.Sprintf("已索引 %d 个物品、%d 个方块、%d 个标签和 %d 个配方", len(catalog.Items), len(catalog.Blocks), len(catalog.Tags), len(catalog.Recipes)))
	return nil
}
