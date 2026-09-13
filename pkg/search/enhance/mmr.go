package enhance

import (
	"math"

	"websearch/pkg/search/core"
)

// ──────────────────────────────────────────────────────────────────────────────
// MMR（Maximal Marginal Relevance）多样性重排
// 在评分流水线阀值过滤之后、maxSize 截断之前执行，
// 打散同一话题的高相似结果（转载站/镜像站/同源博客）。
//
//   MMR(r, S) = λ × relevance(r) − (1 − λ) × max_{s∈S} similarity(r, s)
// ──────────────────────────────────────────────────────────────────────────────

// ApplyMMR 贪心选择 MMR 重排后的结果列表。
// 输入需已按 score 降序排序；得分最高的结果（Top-1）始终保留在首位。
// lambda 为相关性-多样性权衡系数 [0,1]（0 = 纯多样性，1 = 纯相关性）；<=0 时取默认 0.7。
// targetN <= 0 或大于输入长度时按输入长度处理（不截断）。
func ApplyMMR(results []core.SearchResult, lambda float64, targetN int) []core.SearchResult {
	if len(results) <= 1 {
		return results
	}
	if lambda <= 0 {
		lambda = 0.7
	}
	if lambda > 1 {
		lambda = 1
	}
	if targetN <= 0 || targetN > len(results) {
		targetN = len(results)
	}
	if targetN == 1 {
		return results[:1]
	}

	// 预计算所有结果的 token 集合，避免重复分词
	tokenSets := make([]map[string]struct{}, len(results))
	for i, r := range results {
		tokenSets[i] = tokenSet(r.Title + " " + r.Content)
	}

	selected := []int{0} // Top-1 直接保留
	remaining := make([]int, 0, len(results)-1)
	for i := 1; i < len(results); i++ {
		remaining = append(remaining, i)
	}

	for len(selected) < targetN && len(remaining) > 0 {
		bestPos, bestIdx, bestMMR := -1, -1, math.Inf(-1)
		for pos, ri := range remaining {
			maxSim := 0.0
			for _, si := range selected {
				if sim := jaccardSets(tokenSets[ri], tokenSets[si]); sim > maxSim {
					maxSim = sim
				}
			}
			mmr := lambda*results[ri].Score - (1-lambda)*maxSim
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = ri
				bestPos = pos
			}
		}
		selected = append(selected, bestIdx)
		remaining = append(remaining[:bestPos], remaining[bestPos+1:]...)
	}

	out := make([]core.SearchResult, 0, len(selected))
	for _, i := range selected {
		out = append(out, results[i])
	}
	return out
}
