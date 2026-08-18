package rag

import (
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

/*
ExtractText 从指定 PDF 文件中提取纯文本
*/

func ExtractText(filepath string) (string, error) {
	//1.pdf.Open:打开pdf，并返回：f-> 底层文件对象， reader->PDF解析对象
	f, reader, err := pdf.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("open pdf failed:%w", err)
	}
	//defer:把该调用推迟到当前函数返回之前执行，用于关闭或解锁
	defer f.Close()

	//2.通过PDF解析器reader获取纯文本textReader
	/*
		GetPlainText() 返回 io.Reader
		PDF->PDF解析器->一条提供文本数据的数据流
	*/
	textReader, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text failed:%w", err)
	}

	//3.把文本数据流读到内存
	data, err := io.ReadAll(textReader)
	if err != nil {
		return "", fmt.Errorf("read pdf text failed:%w", err)
	}
	return string(data), nil

}
