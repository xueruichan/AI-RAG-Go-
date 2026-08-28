// 把 TopK 搜索结果拼成给大模型看的 Prompt
package rag

import (
	"fmt"
	"strings"

	"gorag/internal/model"
)

// SystemPrompt:
// 每一次调用DeepSeek都会带上的全局规则,限制模型只使用知识库资料，减少幻觉
/*
go中的字符串两种表示：
双引号 "..."：普通字符串，里面的特殊字符（如 \n）会被转义，而且不能跨行。
反引号 `...`：原始字符串字面量，可以跨越多行，内容会原样保留，不处理转义。
*/

const SystemPrompt = `你是企业文档知识库问答助手。

请严格遵守以下规则：

1. 只能依据用户消息中提供的“知识库资料”回答问题。

2. 不要使用资料之外的常识、猜测或你自己的训练知识补全答案。

3. 如果资料不足以支持答案，请明确回答：
“根据当前知识库资料无法确定”。

4. 回答中的关键结论尽量标注对应资料编号，
例如 [资料1]、[资料2]。

5. 回答要准确、简洁，并优先保留资料中的原意。`

// UserPrompt:
// 不只是用户直接提出的问题，而是将用户问题与检索到的知识库资料拼接在一起，形成一个完整的上下文，再交给大模型（LLM）处理。
func BuildUserPrompt(query string, results []model.SearchResult) string {
	// strings.Builder适合连续拼接大量字符串
	var builder strings.Builder

	// Prompt开头
	builder.WriteString("下面是从知识库中检索到的资料：\n\n")

	// 把每一个TopK Chunk写进去
	for i, result := range results {
		//i是从0开始的
		fmt.Fprintf(
			&builder,
			"[资料%d]\n"+
				"来源：%s，chunk_index=%d，chunk_id=%d\n"+
				"内容：\n%s\n\n",
			i+1,
			result.FileName,
			result.ChunkIndex,
			result.ChunkID,
			result.Content,
		)
	}

	//最后追加真正的问题
	builder.WriteString("用户问题：\n")

	builder.WriteString(query)

	builder.WriteString("\n\n请仅根据上面的知识库资料作答。")

	//builder.String() 将 builder 内部累积的所有内容转换成 string 并返回
	return builder.String()
}
