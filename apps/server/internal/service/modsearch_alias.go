package service

import "strings"

/*
模组搜索别名表（2026-10-03，人工核实种子）：玩家搜缩写/社区译名，

	平台元数据里没有这些词（应用能源2 的 Modrinth 标题是 Applied Energistics 2，
	slug 才是 ae2）。key 小写别名；value = Modrinth slug，命中后直取
	/v2/project/{slug} 拿正主。长尾社区译名靠以后接 MC百科，本表只收高频模组。
*/
var modSearchAliases = map[string]string{
	"ae2":   "ae2",
	"应用能源":  "ae2",
	"应用能源2": "ae2",
	"精致存储":  "sophisticated-storage",
	"精致背包":  "sophisticated-backpacks",
	"通用机械":  "mekanism",
	"机械动力":  "create",
	"沉浸工程":  "immersive-engineering",
	"暮色森林":  "the-twilight-forest",
	"农夫乐事":  "farmers-delight",
	"苹果皮":   "appleskin",
	"抽屉":    "storage-drawers",
	"jei":   "jei",
	"rei":   "roughly-enough-items",
	"jade":  "jade",
}

/*
expandModSearchAlias：查询命中别名则返回对应 slug。

	精确命中优先；长度≥3 的查询允许编辑距离 ≤1（"aer" → "ae2"）。
*/
func expandModSearchAlias(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	if slug, ok := modSearchAliases[q]; ok {
		return slug
	}
	if len([]rune(q)) >= 3 {
		for alias, slug := range modSearchAliases {
			if levenshtein(q, strings.ToLower(alias)) <= 1 {
				return slug
			}
		}
	}
	return ""
}

/* levenshtein：标准双行动态规划。别名表量小（十几行），无性能压力。 */
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
