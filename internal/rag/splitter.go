package rag

import (
	"strings"
)

//SplitText 把一整段文本切成多个 Chunk,overlap 指的是相邻两个切片之间互相重叠的字符数量

func SplitText(text string, chunkSize int, overlap int) []string {
	//1.参数检查
	if chunkSize <= 0 {
		//返回空切片
		return []string{}
	}
	if overlap < 0 {
		overlap = 0
	}

	//overlap不能>=chunksize,step=chunksize-overlap，如果大于等于则不能往后移动
	//step是每次往后移动的步数
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}

	//2.string转rune
	//rune本质是int32的别名类型，语义：代表一个unicode码点,按“字符”存储
	//不能简单按 byte 切中文，很可能会把中文字符从中间切开（一个中文占3个字节），导致切出来的是乱码，用rune:按 Unicode code point 为单位切
	runes := []rune(text)

	if len(runes) == 0 {
		return []string{}
	}

	//3.每次向后移动的步数
	//chunks：接收最终的切片
	step := chunkSize - overlap
	chunks := make([]string, 0)

	//最后一个字符的索引是 len(runes)-1，所以要小于而不是小于等于
	for start := 0; start < len(runes); start += step {
		//注：左闭右开所以end要取到真实的右边界后一个位置
		end := start + chunkSize

		//最后一块切片通常不满足长度刚好为chunksize，一般都是小于
		if end > len(runes) {
			end = len(runes)
		}

		//子切片 := 原切片[起始索引:结束索引],左闭右开
		chunk := string(runes[start:end])

		//strings.TrimSpace 是 Go 标准库 strings 包中的一个函数，它会去掉字符串开头和结尾的空白字符
		chunk = strings.TrimSpace(chunk)
		//判断清理后的 chunk 是不是空字符串。如果不是空字符串，就把它追加到 chunks 切片中。
		if chunk != "" {
			//append 是 Go 的内置函数，用于向切片末尾添加元素
			chunks = append(chunks, chunk)
		}

		//判断是否为最后一块切片
		//注：for循环的判断条件只能判断start是否到了末尾，如果没超出就一直切，无法判断当前块是不是最后一个
		if end == len(runes) {
			break
		}
	}

	return chunks
}
