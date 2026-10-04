package store

import (
	"context"
	"fmt"
	"strings"
)

// DisplayNameForProjects resolves provider project ids (CurseForge numeric ids
// and Modrinth slugs) to the display names we already saw from those platforms.
//
// 冲突摘要里直接印 "lhGA9TYQ" 这类外部 ID 对用户没有意义（链路测试缺陷 O8），
// 这里把能查到的名字交回 service 层；查不到的仍要带上 ID，因为那是唯一可用于
// 搜索的线索。只返回非空名字，缺失的 key 由调用方决定降级文案。
func (r *Repository) DisplayNameForProjects(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	seen := make(map[string]bool, len(ids))
	var placeholders []string
	var args []any
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT external_project_id, display_name FROM platform_projects
		 WHERE display_name<>'' AND external_project_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("lookup project names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if _, ok := out[id]; !ok {
			out[id] = name
		}
	}
	return out, rows.Err()
}

// PlatformProjectBrief is the display snapshot of one platform_projects row.
// slug 用于中文名反查（别名表是「中文别名 → slug」），description 是模组树
// 展开后第一行的一句话描述（0029 起落库）。
type PlatformProjectBrief struct {
	Platform, ExternalProjectID, Slug, Description string
}

// PlatformProjectBriefs 按 external_project_id 批量取平台项目展示字段。
// key = platform + "|" + external_project_id：CF 的数字 ID 与 Modrinth 的
// base62 空间不同，但 key 带上平台才不会在理论撞车时串数据。
func (r *Repository) PlatformProjectBriefs(ctx context.Context, externalIDs []string) (map[string]PlatformProjectBrief, error) {
	out := make(map[string]PlatformProjectBrief, len(externalIDs))
	seen := make(map[string]bool, len(externalIDs))
	var placeholders []string
	var args []any
	for _, id := range externalIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT platform, external_project_id, COALESCE(slug,''), COALESCE(description,'') FROM platform_projects
		 WHERE external_project_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("lookup project briefs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b PlatformProjectBrief
		if err := rows.Scan(&b.Platform, &b.ExternalProjectID, &b.Slug, &b.Description); err != nil {
			return nil, err
		}
		out[b.Platform+"|"+b.ExternalProjectID] = b
	}
	return out, rows.Err()
}

// SetPlatformProjectDescription 回写补拉到的一句话描述。空串不写（没信息量，
// 还会覆盖并发写入的真值）；行不存在（还没 upsert 过）时影响 0 行，无害。
func (r *Repository) SetPlatformProjectDescription(ctx context.Context, platform, externalProjectID, description string) error {
	if strings.TrimSpace(description) == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE platform_projects SET description=? WHERE platform=? AND external_project_id=?`,
		description, platform, externalProjectID)
	if err != nil {
		return fmt.Errorf("update project description: %w", err)
	}
	return nil
}

// ModContentKindCounts 统计一个包内每个模组每种解析产物的精确条数。
// 模组树要用它做两件事：展开时的分类计数（不用再靠前端对 1000 条取样数）
// 和「空模组」判定 —— 一个模组除了语言/元数据什么内容都不贡献时，列表里
// 折叠成一行就好，不用给它一个永远空的展开区。
func (r *Repository) ModContentKindCounts(ctx context.Context, packID string) (map[string]map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mod_id, kind, COUNT(*) FROM mod_content WHERE pack_id=? GROUP BY mod_id, kind`, packID)
	if err != nil {
		return nil, fmt.Errorf("count mod content kinds: %w", err)
	}
	defer rows.Close()
	out := make(map[string]map[string]int64)
	for rows.Next() {
		var modID, kind string
		var n int64
		if err := rows.Scan(&modID, &kind, &n); err != nil {
			return nil, err
		}
		if out[modID] == nil {
			out[modID] = make(map[string]int64)
		}
		out[modID][kind] = n
	}
	return out, rows.Err()
}
