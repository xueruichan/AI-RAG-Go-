
"use strict";

// ============================================================
// 0. 获取页面元素
// ============================================================

// 因为前端由 Gin 在同一个域名下提供，API 使用相对路径即可。
// 例如 fetch("/documents") 会访问当前服务器的 /documents。
const $ = (selector) => document.querySelector(selector);

const fileInput = $("#fileInput");
const chooseFileBtn = $("#chooseFileBtn");
const selectedFileName = $("#selectedFileName");
const uploadBtn = $("#uploadBtn");
const uploadStatus = $("#uploadStatus");
const documentsList = $("#documentsList");
const refreshDocsBtn = $("#refreshDocsBtn");

const apiStatus = $("#apiStatus");
const apiStatusText = $("#apiStatusText");

const chatForm = $("#chatForm");
const questionInput = $("#questionInput");
const chatMessages = $("#chatMessages");
const sendBtn = $("#sendBtn");
const stopBtn = $("#stopBtn");

const searchForm = $("#searchForm");
const searchInput = $("#searchInput");
const searchTopK = $("#searchTopK");
const searchBtn = $("#searchBtn");
const searchStatus = $("#searchStatus");
const searchResults = $("#searchResults");

// 当前选中的 PDF。
let selectedFile = null;

// 当前是否正在进行流式问答。
let isSending = false;

// 用于停止当前 SSE 请求。
let activeController = null;


// ============================================================
// 1. 通用 HTTP 请求与错误处理
// ============================================================

// 将响应正文读取为 JSON。
// 后端返回非 2xx 状态码时，尽可能提取 error 字段。
async function requestJSON(url, options = {}) {
    const response = await fetch(url, options);
    const text = await response.text();

    let data = null;

    if (text) {
        try {
            data = JSON.parse(text);
        } catch {
            data = { message: text };
        }
    }

    if (!response.ok) {
        const message =
            data?.error ||
            data?.message ||
            `HTTP ${response.status}`;

        throw new Error(message);
    }

    return data;
}

// 用于上传状态轮询时暂停一会儿。
function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

// 更新左上角的后端连接状态。
function setAPIStatus(online, message) {
    apiStatus.classList.toggle("online", online);
    apiStatus.classList.toggle("offline", !online);
    apiStatusText.textContent = message;
}

// 更新上传区域的提示文字。
function setUploadStatus(message, type = "") {
    uploadStatus.textContent = message;
    uploadStatus.className = "upload-status";

    if (type) {
        uploadStatus.classList.add(type);
    }
}


// ============================================================
// 2. 检查后端健康状态
// ============================================================

async function checkHealth() {
    try {
        // 对应你已有的 GET /health。
        const result = await requestJSON("/health");

        if (result?.code === 0) {
            setAPIStatus(true, "服务已连接");
        } else {
            setAPIStatus(true, "服务可访问");
        }
    } catch (error) {
        setAPIStatus(false, "服务连接失败");
        console.error("健康检查失败：", error);
    }
}


// ============================================================
// 3. 文档管理：列表、上传、轮询、删除
// ============================================================

// 将文档状态转换为前端显示文字。
function getDocumentStatus(status) {
    const normalized = String(status || "").toUpperCase();

    const statusMap = {
        PROCESSING: ["处理中", "processing"],
        SUCCESS: ["已完成", "success"],
        FAILED: ["处理失败", "failed"],
        PENDING: ["等待处理", "processing"]
    };

    return statusMap[normalized] || [normalized || "未知状态", ""];
}

// 创建文档列表中的一张卡片。
// 动态数据全部使用 textContent，避免将文件名当作 HTML 执行。
function createDocumentItem(doc) {
    const item = document.createElement("article");
    item.className = "document-item";

    const top = document.createElement("div");
    top.className = "document-top";

    const icon = document.createElement("div");
    icon.className = "document-icon";
    icon.textContent = "PDF";

    const info = document.createElement("div");
    info.className = "document-info";

    const name = document.createElement("div");
    name.className = "document-name";
    name.textContent = doc.filename || "未命名文档";

    const id = document.createElement("div");
    id.className = "document-id";
    id.textContent = `文档 ID：${doc.id}`;

    info.append(name, id);
    top.append(icon, info);

    const bottom = document.createElement("div");
    bottom.className = "document-bottom";

    const status = document.createElement("span");
    const [statusText, statusClass] = getDocumentStatus(doc.status);

    status.className = `document-status ${statusClass}`;
    status.textContent = statusText;

    const deleteBtn = document.createElement("button");
    deleteBtn.className = "delete-document";
    deleteBtn.type = "button";
    deleteBtn.textContent = "删除";

    // 删除操作需要用户确认，防止误删文档。
    deleteBtn.addEventListener("click", () => {
        deleteDocument(doc.id, doc.filename);
    });

    bottom.append(status, deleteBtn);
    item.append(top, bottom);

    return item;
}

// GET /documents：刷新整个文档列表。
async function loadDocuments() {
    documentsList.replaceChildren();

    const loading = document.createElement("div");
    loading.className = "empty-state";
    loading.textContent = "正在加载文档...";
    documentsList.append(loading);

    try {
        const documents = await requestJSON("/documents");

        if (!Array.isArray(documents) || documents.length === 0) {
            documentsList.replaceChildren();

            const empty = document.createElement("div");
            empty.className = "empty-state";
            empty.textContent = "还没有文档，先上传一份 PDF 吧。";

            documentsList.append(empty);
            return;
        }

        // 最新上传的文档排在前面。
        documents.sort((a, b) => {
            return new Date(b.created_at || 0) -
                   new Date(a.created_at || 0);
        });

        documentsList.replaceChildren();

        for (const doc of documents) {
            documentsList.append(createDocumentItem(doc));
        }
    } catch (error) {
        documentsList.replaceChildren();

        const failed = document.createElement("div");
        failed.className = "empty-state";
        failed.textContent = `加载文档失败：${error.message}`;

        documentsList.append(failed);
        console.error("加载文档失败：", error);
    }
}

// 用户选择 PDF 文件。
chooseFileBtn.addEventListener("click", () => {
    fileInput.click();
});

fileInput.addEventListener("change", () => {
    selectedFile = fileInput.files?.[0] || null;

    if (!selectedFile) {
        selectedFileName.textContent = "尚未选择文件";
        uploadBtn.disabled = true;
        return;
    }

    selectedFileName.textContent =
        `${selectedFile.name} · ${(selectedFile.size / 1024).toFixed(1)} KB`;

    const isPDF = selectedFile.name.toLowerCase().endsWith(".pdf");
    const isValidSize = selectedFile.size > 0 &&
                        selectedFile.size <= 10 * 1024 * 1024;

    uploadBtn.disabled = !(isPDF && isValidSize);

    if (!isPDF) {
        setUploadStatus("请选择 PDF 文件。", "error");
    } else if (!isValidSize) {
        setUploadStatus("文件不能为空，且不能超过 10 MB。", "error");
    } else {
        setUploadStatus("文件已准备好，可以上传。");
    }
});

// POST /documents：使用 FormData 上传 PDF。
uploadBtn.addEventListener("click", uploadDocument);

async function uploadDocument() {
    if (!selectedFile || uploadBtn.disabled) {
        return;
    }

    uploadBtn.disabled = true;
    chooseFileBtn.disabled = true;

    setUploadStatus("正在上传 PDF...");

    try {
        // 字段名必须是 file，因为后端使用 c.FormFile("file")。
        const formData = new FormData();
        formData.append("file", selectedFile);

        const result = await requestJSON("/documents", {
            method: "POST",
            body: formData
        });

        const documentID = result.document_id;

        setUploadStatus(
            `上传成功，文档 ID：${documentID}，正在后台处理...`,
            "success"
        );

        // 先刷新列表，让新文档尽快显示出来。
        await loadDocuments();

        // 上传接口返回 202 只代表接收成功。
        // 继续轮询详情，直到 Worker 处理完成。
        await waitForDocument(documentID);

        // 完成后清空文件选择框。
        selectedFile = null;
        fileInput.value = "";
        selectedFileName.textContent = "尚未选择文件";
    } catch (error) {
        setUploadStatus(`上传失败：${error.message}`, "error");
        console.error("上传失败：", error);
    } finally {
        chooseFileBtn.disabled = false;
        uploadBtn.disabled = !selectedFile;
    }
}

// 通过 GET /documents/:id 轮询后台处理状态。
// 每 3 秒检查一次，最多等待 60 秒。
async function waitForDocument(documentID) {
    for (let i = 0; i < 20; i++) {
        await sleep(3000);

        try {
            const doc = await requestJSON(`/documents/${documentID}`);
            const status = String(doc.status || "").toUpperCase();

            if (status === "SUCCESS") {
                setUploadStatus(
                    `处理完成！文档 ID：${documentID}`,
                    "success"
                );
                await loadDocuments();
                return;
            }

            if (status === "FAILED") {
                setUploadStatus(
                    `PDF 处理失败，请查看 GoRAG 服务日志。文档 ID：${documentID}`,
                    "error"
                );
                await loadDocuments();
                return;
            }

            setUploadStatus(
                `文档 ID：${documentID}，后台处理中...（${i + 1}/20）`
            );
        } catch (error) {
            setUploadStatus(`查询处理状态失败：${error.message}`, "error");
            return;
        }
    }

    setUploadStatus(
        `文档 ID：${documentID} 仍在处理中，可点击刷新查看状态。`
    );

    await loadDocuments();
}

// DELETE /documents/:id：删除指定文档。
async function deleteDocument(id, filename) {
    const confirmed = window.confirm(
        `确定要删除文档「${filename}」吗？\n对应的文档记录和向量也会尝试清理。`
    );

    if (!confirmed) {
        return;
    }

    try {
        await requestJSON(`/documents/${id}`, {
            method: "DELETE"
        });

        setUploadStatus(`已请求删除文档 ${id}。`, "success");
        await loadDocuments();
    } catch (error) {
        setUploadStatus(`删除失败：${error.message}`, "error");
    }
}

// 点击刷新按钮。
refreshDocsBtn.addEventListener("click", loadDocuments);


// ============================================================
// 4. 聊天消息与来源卡片
// ============================================================

// 创建一条用户消息或助手消息。
// 返回对应的 DOM 节点，后续可以持续追加流式文本。
function appendMessage(role, initialText = "") {
    // 第一次开始聊天时，移除欢迎页面。
    const welcome = $("#welcomeCard");
    if (welcome) {
        welcome.remove();
    }

    const row = document.createElement("article");
    row.className = `message ${role}`;

    const avatar = document.createElement("div");
    avatar.className = "message-avatar";
    avatar.textContent = role === "user" ? "你" : "G";

    const content = document.createElement("div");
    content.className = "message-content";

    const meta = document.createElement("div");
    meta.className = "message-meta";
    meta.textContent = role === "user" ? "你" : "GoRAG 助手";

    const bubble = document.createElement("div");
    bubble.className = "message-bubble";

    const text = document.createElement("div");
    text.className = "message-text";
    text.textContent = initialText;

    const sources = document.createElement("div");
    sources.className = "source-list";
    sources.hidden = true;

    bubble.append(text, sources);
    content.append(meta, bubble);
    row.append(avatar, content);
    chatMessages.append(row);

    chatMessages.scrollTop = chatMessages.scrollHeight;

    return { row, text, sources, meta };
}

// 把后端返回的来源列表显示在助手消息下面。
function renderSources(container, sources) {
    container.replaceChildren();

    if (!Array.isArray(sources) || sources.length === 0) {
        container.hidden = true;
        return;
    }

    container.hidden = false;

    const heading = document.createElement("div");
    heading.className = "source-heading";
    heading.textContent = `参考资料 · ${sources.length} 条`;
    container.append(heading);

    for (const source of sources) {
        const card = document.createElement("div");
        card.className = "source-card";

        const title = document.createElement("strong");
        title.textContent =
            `${source.reference || "资料"} · ${source.filename || "未知文件"}`;

        const detail = document.createElement("div");
        detail.textContent =
            `文档 ID：${source.document_id} · ` +
            `Chunk：${source.chunk_index} · ` +
            `相似度：${Number(source.score || 0).toFixed(3)}`;

        card.append(title, detail);
        container.append(card);
    }
}

// 切换发送状态，防止同一时间提交多条请求。
function setSending(sending) {
    isSending = sending;
    sendBtn.disabled = sending;
    stopBtn.hidden = !sending;
    questionInput.disabled = sending;
}


// ============================================================
// 5. SSE 解析：将网络字节流解析为完整事件
// ============================================================

// SSE 的基本格式：
//
// event: delta
// data: {"content":"你好"}
//
// 空行表示一个事件结束。
//
// 注意：一次网络读取不一定刚好对应一个完整事件。
// 一个事件可能被拆成多个网络块，因此需要缓冲区。
function parseSSEFrame(frame, onEvent) {
    let eventName = "message";
    const dataLines = [];

    for (const rawLine of frame.split("\n")) {
        const line = rawLine.replace(/\r$/, "");

        // 冒号开头的是 SSE 注释，不需要处理。
        if (!line || line.startsWith(":")) {
            continue;
        }

        const separator = line.indexOf(":");

        const field = separator < 0
            ? line
            : line.slice(0, separator);

        let value = separator < 0
            ? ""
            : line.slice(separator + 1);

        // SSE 允许冒号后有一个空格。
        if (value.startsWith(" ")) {
            value = value.slice(1);
        }

        if (field === "event") {
            eventName = value;
        } else if (field === "data") {
            dataLines.push(value);
        }
    }

    if (dataLines.length === 0) {
        return;
    }

    // 多个 data 行按照 SSE 规则拼接。
    const rawData = dataLines.join("\n");

    let payload = rawData;

    // 你的 Go 后端在 data 中发送 JSON，
    // 因此尝试将其解析为 JavaScript 对象。
    try {
        payload = JSON.parse(rawData);
    } catch {
        // 如果不是 JSON，就保留原始字符串。
    }

    onEvent(eventName, payload);
}

// 使用 fetch + ReadableStream 消费 POST SSE。
// 不能直接用原生 EventSource，因为本项目的 SSE
// 接口需要 POST 请求体中的 query 和 top_k。
async function consumeSSE(response, onEvent) {
    if (!response.body) {
        throw new Error("浏览器不支持读取该响应流");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder("utf-8");

    // 保存尚未拼成完整 SSE 事件的数据。
    let buffer = "";

    while (true) {
        const { value, done } = await reader.read();

        // stream:true 保证 UTF-8 多字节字符被拆开时，
        // 不会在中间解码成乱码。
        buffer += decoder.decode(value, { stream: !done });

        // 兼容 Windows 风格的 CRLF 换行。
        buffer = buffer.replace(/\r\n/g, "\n");

        let boundary;

        // 处理缓冲区中已经完整到达的事件。
        while ((boundary = buffer.indexOf("\n\n")) !== -1) {
            const frame = buffer.slice(0, boundary);
            buffer = buffer.slice(boundary + 2);

            parseSSEFrame(frame, onEvent);
        }

        if (done) {
            break;
        }
    }

    // 处理流结束时可能剩余的最后一个事件。
    if (buffer.trim()) {
        parseSSEFrame(buffer, onEvent);
    }
}


// ============================================================
// 6. SSE 流式问答
// ============================================================

chatForm.addEventListener("submit", async (event) => {
    event.preventDefault();

    const query = questionInput.value.trim();

    if (!query || isSending) {
        return;
    }

    // 显示用户的问题。
    appendMessage("user", query);

    // 先创建一个空的助手消息。
    // 后续每收到 delta，就更新这一条消息，
    // 而不是每个片段都创建一条新的消息。
    const assistant = appendMessage("assistant");

    questionInput.value = "";
    questionInput.style.height = "auto";

    setSending(true);

    // AbortController 允许用户主动停止生成。
    activeController = new AbortController();

    let hasDelta = false;
    let finished = false;

    try {
        const response = await fetch("/ask/stream", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                query,
                top_k: 5
            }),
            signal: activeController.signal
        });

        // 处理 SSE 开始前的 HTTP 错误，例如 400 或 500。
        if (!response.ok) {
            const text = await response.text();
            let message = text;

            try {
                const data = JSON.parse(text);
                message = data.error || data.message || text;
            } catch {
                // 错误正文不是 JSON 时，直接显示原始文本。
            }

            throw new Error(message || `HTTP ${response.status}`);
        }

        const contentType = response.headers.get("content-type") || "";

        if (!contentType.includes("text/event-stream")) {
            throw new Error(
                `预期 SSE 响应，实际 Content-Type：${contentType || "未知"}`
            );
        }

        // 开始读取后端持续返回的 SSE 事件。
        await consumeSSE(response, (eventName, data) => {

            // 事件一：后端开始检索。
            if (eventName === "status") {
                assistant.meta.textContent = "正在检索知识库...";
                assistant.text.textContent = "正在检索相关资料...";
            }

            // 事件二：后端返回检索来源。
            else if (eventName === "sources") {
                renderSources(
                    assistant.sources,
                    data.sources || []
                );

                assistant.meta.textContent =
                    `已检索到 ${(data.sources || []).length} 条参考资料`;

                assistant.text.textContent = "已找到相关资料，正在生成回答...";
            }

            // 事件三：大模型逐段返回文本。
            else if (eventName === "delta") {
                const content = data.content || "";

                if (!hasDelta) {
                    // 第一个文本片段到达时，清除等待提示。
                    assistant.text.textContent = "";
                    hasDelta = true;
                }

                // 每次只追加新文本，不覆盖已经生成的内容。
                assistant.text.textContent += content;

                assistant.meta.textContent = "正在生成回答...";
                chatMessages.scrollTop = chatMessages.scrollHeight;
            }

            // 事件四：正常完成。
            else if (eventName === "done") {
                finished = true;
                assistant.meta.textContent = "回答完成";
            }

            // 事件五：生成过程中发生错误。
            else if (eventName === "error") {
                throw new Error(data.error || "流式问答发生错误");
            }
        });

        if (finished) {
            assistant.meta.textContent = "回答完成";
        } else if (!hasDelta) {
            assistant.text.textContent =
                "连接已结束，但没有收到答案文本。请检查后端 SSE 日志。";
        } else {
            assistant.meta.textContent = "连接已结束";
        }

    } catch (error) {
        if (error.name === "AbortError") {
            assistant.meta.textContent = "已停止生成";

            if (!assistant.text.textContent) {
                assistant.text.textContent = "本次生成已停止。";
            }
        } else {
            assistant.meta.textContent = "请求失败";
            assistant.text.textContent = `问答失败：${error.message}`;
            console.error("SSE 问答失败：", error);
        }
    } finally {
        activeController = null;
        setSending(false);
        questionInput.focus();
        chatMessages.scrollTop = chatMessages.scrollHeight;
    }
});

// 主动取消当前请求。
stopBtn.addEventListener("click", () => {
    if (activeController) {
        activeController.abort();
    }
});

// Enter 发送，Shift + Enter 换行。
questionInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        chatForm.requestSubmit();
    }
});

// 根据文本内容自动调整输入框高度。
questionInput.addEventListener("input", () => {
    questionInput.style.height = "auto";
    questionInput.style.height =
        `${Math.min(questionInput.scrollHeight, 140)}px`;
});

// 点击示例问题，自动填入输入框。
document.querySelectorAll(".suggestion-button").forEach((button) => {
    button.addEventListener("click", () => {
        questionInput.value = button.dataset.question || "";
        questionInput.focus();
    });
});


// ============================================================
// 7. 语义检索页面
// ============================================================

// 创建一个检索结果卡片。
function createSearchResult(result) {
    const card = document.createElement("article");
    card.className = "result-card";

    const header = document.createElement("div");
    header.className = "result-header";

    const filename = document.createElement("div");
    filename.className = "result-filename";
    filename.textContent = result.filename || "未知文档";

    const score = document.createElement("span");
    score.className = "score-badge";
    score.textContent =
        `相似度 ${Number(result.score || 0).toFixed(3)}`;

    header.append(filename, score);

    const content = document.createElement("div");
    content.className = "result-content";
    content.textContent = result.content || "没有文本内容";

    const footer = document.createElement("div");
    footer.className = "result-footer";
    footer.textContent =
        `文档 ID：${result.document_id} · ` +
        `Chunk ID：${result.chunk_id} · ` +
        `Chunk 序号：${result.chunk_index}`;

    card.append(header, content, footer);

    return card;
}

// POST /search：独立检索，不调用大模型生成答案。
searchForm.addEventListener("submit", async (event) => {
    event.preventDefault();

    const query = searchInput.value.trim();

    if (!query) {
        searchStatus.textContent = "请输入要检索的内容。";
        return;
    }

    searchBtn.disabled = true;
    searchResults.replaceChildren();
    searchStatus.textContent = "正在检索知识库...";

    try {
        const results = await requestJSON("/search", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                query,
                top_k: Number(searchTopK.value)
            })
        });

        if (!Array.isArray(results) || results.length === 0) {
            searchStatus.textContent = "没有找到相关文本块。";
            return;
        }

        searchStatus.textContent = `检索完成，共找到 ${results.length} 条结果。`;

        for (const result of results) {
            searchResults.append(createSearchResult(result));
        }
    } catch (error) {
        searchStatus.textContent = `检索失败：${error.message}`;
        console.error("语义检索失败：", error);
    } finally {
        searchBtn.disabled = false;
    }
});


// ============================================================
// 8. 页面标签切换
// ============================================================

document.querySelectorAll(".tab-button").forEach((button) => {
    button.addEventListener("click", () => {
        const targetID = button.dataset.view;

        // 切换按钮的选中样式。
        document.querySelectorAll(".tab-button").forEach((item) => {
            item.classList.toggle("active", item === button);
        });

        // 只显示当前选中的功能区域。
        document.querySelectorAll(".workspace-view").forEach((view) => {
            view.hidden = view.id !== targetID;
        });
    });
});


// ============================================================
// 9. 页面启动
// ============================================================

// 页面加载完成后，先检查服务状态并获取文档列表。
checkHealth();
loadDocuments();
