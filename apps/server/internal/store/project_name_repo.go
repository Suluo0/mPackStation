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
