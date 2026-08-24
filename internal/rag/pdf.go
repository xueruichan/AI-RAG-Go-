package rag

import (
	"fmt"
	"os/exec"
	"strings"
)

/*
ExtractText:

使用系统 pdftotext 提取 PDF 文本。

原因：

很多 PDF（尤其 Word 导出的 PDF）
内部字体编码复杂。

Go 部分 PDF 库解析可能得到：

\u0000
乱码
错误字符

而 poppler 的 pdftotext
对这类 PDF 兼容性更好。
*/
func ExtractText(filepath string) (string, error) {

	/*
		执行：

		pdftotext xxx.pdf -

		最后的 "-" 表示：

		输出到标准输出 stdout

		例如：

		PDF
		 |
		 pdftotext
		 |
		output byte
	*/
	cmd := exec.Command(
		"pdftotext",
		filepath,
		"-",
	)

	output, err := cmd.Output()

	if err != nil {

		return "",
			fmt.Errorf(
				"extract pdf text failed: %w",
				err,
			)
	}

	// byte 转 string
	text := string(output)

	/*
		文本有效性检查：

		避免：

		空PDF
		扫描PDF
		无文字PDF

		进入：

		Chunk
		 |
		Embedding
		 |
		Milvus

		产生无意义向量。
	*/
	if len(strings.TrimSpace(text)) == 0 {

		return "",
			fmt.Errorf(
				"pdf contains no text",
			)
	}

	return text, nil
}
