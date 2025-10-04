// 全局单一 SSE 连接与轮询器（尽量减少 HTTP/1.1 长连接）
window.globalSSE = null;
window.pollTimer = null;
window.toolsData = [];

function ensureSSE() {
    if (window.globalSSE) return window.globalSSE;
    // 建立单一 SSE 连接，接收所有工具的进度
    const es = new EventSource('api/progress');
    window.globalSSE = es;

    es.onmessage = function (event) {
        const progress = JSON.parse(event.data);
        updateProgress(progress);
        // 任一任务结束后，检查后端是否还有活跃任务；若无则关闭 SSE
        if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'uninstalled') {
            maybeStopSSEIfNoActive();
        }
    };

    es.onerror = function () {
        console.error('SSE connection error');
        closeSSE();
        startPollingActive();
    };

    // 连接建立后可停止轮询
    stopPollingActive();
    return es;
}

function closeSSE() {
    if (window.globalSSE) {
        try { window.globalSSE.close(); } catch (e) { }
        window.globalSSE = null;
    }
}

async function maybeStopSSEIfNoActive() {
    try {
        const resp = await fetch('api/active');
        const data = await resp.json();
        if (!data.needsSSE) {
            closeSSE();
            startPollingActive();
        }
    } catch (e) {
        // 网络异常时保守不关闭 SSE
    }
}

function startPollingActive(intervalMs = 5000) {
    if (window.pollTimer || window.globalSSE) return; // 已有轮询或已经连上
    window.pollTimer = setInterval(async function () {
        try {
            const resp = await fetch('api/active');
            const data = await resp.json();
            if (data.needsSSE) {
                ensureSSE();
            }
        } catch (e) {
            // 忽略，下一次轮询再试
        }
    }, intervalMs);
}

function stopPollingActive() {
    if (window.pollTimer) {
        clearInterval(window.pollTimer);
        window.pollTimer = null;
    }
}

// Load tools list
async function loadTools() {
    try {
        const response = await fetch('api/tools');
        const tools = await response.json();

        window.toolsData = tools;
        renderTools(tools);
    } catch (error) {
        document.getElementById('tools-container').innerHTML =
            '<div class="error-message" style="display:block;">' + t('failedToLoad') + ': ' + error.message + '</div>';
    }
}

// Render tools grid
function renderTools(tools) {
    const container = document.getElementById('tools-container');
    container.className = 'tools-grid';
    container.innerHTML = '';

    // 兜底排序：名称升序，同名按语义化/字符串版本升序
    const semverCmp = (a, b) => {
        // 简单拆分比较：主.次.补丁，非严格；前端仅兜底
        const pa = (a || '').replace(/^v/i, '').split('.').map(x => parseInt(x, 10));
        const pb = (b || '').replace(/^v/i, '').split('.').map(x => parseInt(x, 10));
        for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
            const ai = isNaN(pa[i]) ? 0 : pa[i];
            const bi = isNaN(pb[i]) ? 0 : pb[i];
            if (ai !== bi) return ai - bi;
        }
        return 0;
    };
    tools = tools.slice().sort((x, y) => {
        if (x.name !== y.name) return x.name < y.name ? -1 : 1;
        const c = semverCmp(x.version, y.version);
        return c === 0 ? 0 : (c < 0 ? -1 : 1);
    });

    tools.forEach(tool => {
        const card = createToolCard(tool);
        container.appendChild(card);
    });
}

// Create tool card element using template
function createToolCard(tool) {
    const template = document.getElementById('tool-card-template');
    const clone = template.content.cloneNode(true);

    const card = clone.querySelector('.tool-card');
    card.id = 'tool-' + tool.name + '-' + tool.version;

    // Set tool name
    const toolNameEl = clone.querySelector('.tool-name');
    toolNameEl.textContent = tool.name;

    // Set tool version
    const toolVersionEl = clone.querySelector('.tool-version');
    toolVersionEl.textContent = t('version') + ': ' + tool.version;

    // Set status
    const statusEl = clone.querySelector('.tool-status');
    // Folder and info
    const folderBtnEl = clone.querySelector('.folder-btn');
    const infoBtnEl = clone.querySelector('.info-btn');
    const folderPanelEl = clone.querySelector('.folder-panel');
    const infoPanelEl = clone.querySelector('.info-panel');
    folderBtnEl.textContent = t('folder');
    infoBtnEl.textContent = t('showInfo');
    // 信息缓存标记存放到 card dataset，便于全局事件重置
    card.dataset.infoLoaded = 'false';

    // 工具方法：互斥展开
    function toggleExclusive(showEl, hideEl, hideBtn) {
        const isOpen = showEl.style.display === 'block';
        // 先收起两个
        showEl.style.display = 'none';
        hideEl.style.display = 'none';
        // 按钮状态复位
        folderBtnEl.classList.remove('active');
        infoBtnEl.classList.remove('active');
        if (hideBtn) hideBtn.textContent = t('showInfo');
        // 再按需展开
        if (!isOpen) {
            showEl.style.display = 'block';
            // 根据展开的面板，高亮对应按钮
            if (showEl === folderPanelEl) {
                folderBtnEl.classList.add('active');
            } else if (showEl === infoPanelEl) {
                infoBtnEl.classList.add('active');
            }
        }
    }

    // 加载目录：首次点击再加载，减少无谓请求
    let folderLoaded = false;
    folderBtnEl.addEventListener('click', async function () {
        toggleExclusive(folderPanelEl, infoPanelEl, infoBtnEl);
        if (!folderLoaded && folderPanelEl.style.display === 'block') {
            try {
                const resp = await fetch(`api/tool-path?toolName=${encodeURIComponent(tool.name)}&version=${encodeURIComponent(tool.version)}`);
                if (resp.ok) {
                    const data = await resp.json();
                    if (data && typeof data.path === 'string') {
                        folderPanelEl.textContent = data.path;
                        folderLoaded = true;
                    }
                }
            } catch (e) { /* ignore */ }
        }
    });

    // 绑定 info 按钮：首次点击拉取，后续切换只隐藏/展示
    infoBtnEl.addEventListener('click', async function () {
        const willOpen = infoPanelEl.style.display !== 'block';
        toggleExclusive(infoPanelEl, folderPanelEl, null);
        if (willOpen) {
            infoBtnEl.textContent = t('hideInfo');
        } else {
            infoBtnEl.textContent = t('showInfo');
        }
        if (card.dataset.infoLoaded !== 'true' && infoPanelEl.style.display === 'block') {
            try {
                const resp = await fetch(`api/tool-info?toolName=${encodeURIComponent(tool.name)}&version=${encodeURIComponent(tool.version)}`);
                if (resp.ok) {
                    const data = await resp.json();
                    infoPanelEl.textContent = (data && typeof data.info === 'string') ? data.info : '';
                    card.dataset.infoLoaded = 'true';
                } else {
                    infoPanelEl.textContent = '';
                }
            } catch (e) {
                infoPanelEl.textContent = '';
            }
        }
    });
    const statusClass = tool.installed ? 'status-installed' : 'status-not-installed';
    const statusText = tool.installed ? t('installed') : t('notInstalled');
    statusEl.className = 'tool-status ' + statusClass;
    statusEl.textContent = statusText;

    // Set install button - only show for non-installed tools
    const installBtnEl = clone.querySelector('.install-btn');
    const pauseBtnEl = clone.querySelector('.pause-btn');
    const resumeBtnEl = clone.querySelector('.resume-btn');
    const uninstallBtnEl = clone.querySelector('.uninstall-btn');

    installBtnEl.textContent = t('install');
    pauseBtnEl.textContent = t('pause');
    resumeBtnEl.textContent = t('resume');
    uninstallBtnEl.textContent = t('uninstall');

    installBtnEl.addEventListener('click', async function () {
        try {
            await installTool(tool.name, tool.version);
            // 触发安装时先重置 info 缓存，避免旧信息误用
            const parentCard = document.getElementById('tool-' + tool.name + '-' + tool.version);
            if (parentCard) {
                parentCard.dataset.infoLoaded = 'false';
                const p = parentCard.querySelector('.info-panel');
                const b = parentCard.querySelector('.info-btn');
                if (p && b) { p.textContent = ''; p.style.display = 'none'; b.textContent = t('showInfo'); b.classList.remove('active'); }
            }
        } catch (error) {
            console.error(error);
        }
    });
    pauseBtnEl.addEventListener('click', async function () {
        try {
            await pauseDownload(tool.name, tool.version);
        } catch (error) {
            console.error(error);
        }
    });
    resumeBtnEl.addEventListener('click', async function () {
        try {
            await installTool(tool.name, tool.version);
        } catch (error) {
            console.error(error);
        }
    });
    uninstallBtnEl.addEventListener('click', async function () {
        try {
            await uninstallTool(tool.name, tool.version);
            // 卸载触发时也重置 info 缓存
            const parentCard = document.getElementById('tool-' + tool.name + '-' + tool.version);
            if (parentCard) {
                parentCard.dataset.infoLoaded = 'false';
                const p = parentCard.querySelector('.info-panel');
                const b = parentCard.querySelector('.info-btn');
                if (p && b) { p.textContent = ''; p.style.display = 'none'; b.textContent = t('showInfo'); b.classList.remove('active'); }
            }
        } catch (error) {
            console.error(error);
        }
    });

    if (tool.installed) {
        // Hide install button, show uninstall button
        installBtnEl.style.display = 'none';
        pauseBtnEl.style.display = 'none';
        resumeBtnEl.style.display = 'none';
    } else {
        // Show install button, hide uninstall button
        uninstallBtnEl.style.display = 'none';
        pauseBtnEl.style.display = 'none';
        resumeBtnEl.style.display = 'none';
    }

    return clone;
}

// Install tool
async function installTool(toolName, version) {
    const cardId = 'tool-' + toolName + '-' + version;
    const card = document.getElementById(cardId);
    const installBtn = card.querySelector('.install-btn');
    const pauseBtn = card.querySelector('.pause-btn');
    const resumeBtn = card.querySelector('.resume-btn');
    const progressContainer = card.querySelector('.progress-container');
    const errorMessage = card.querySelector('.error-message');
    const statusDiv = card.querySelector('.tool-status');

    // Reset UI
    installBtn.disabled = true;
    progressContainer.style.display = 'block';
    errorMessage.style.display = 'none';
    statusDiv.className = 'tool-status status-installing';
    statusDiv.textContent = t('installing');

    try {
        // 点击安装后确保建立 SSE
        ensureSSE();
        const response = await fetch('api/install', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ toolName, version })
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        // 安装任务已开始：隐藏“安装”，展示“暂停”；等待 SSE 的 completed 再切为卸载
        installBtn.style.display = 'none';
        pauseBtn.style.display = 'inline-block';
        pauseBtn.disabled = false;
        resumeBtn.style.display = 'none';
    } catch (error) {
        errorMessage.textContent = t('error') + ': ' + error.message;
        errorMessage.style.display = 'block';
        installBtn.disabled = false;
        progressContainer.style.display = 'none';
        statusDiv.className = 'tool-status status-not-installed';
        statusDiv.textContent = t('failed');
    }
}

// Uninstall tool
async function uninstallTool(toolName, version) {
    const cardId = 'tool-' + toolName + '-' + version;
    const card = document.getElementById(cardId);
    const uninstallBtn = card.querySelector('.uninstall-btn');
    const pauseBtn = card.querySelector('.pause-btn');
    const resumeBtn = card.querySelector('.resume-btn');
    const errorMessage = card.querySelector('.error-message');
    const statusDiv = card.querySelector('.tool-status');

    // Reset UI
    uninstallBtn.disabled = true;
    errorMessage.style.display = 'none';
    statusDiv.className = 'tool-status status-installing';
    statusDiv.textContent = t('uninstalling');

    try {
        // 点击卸载后确保建立 SSE，以便接收 "uninstalled" 消息
        ensureSSE();
        const response = await fetch('api/uninstall', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ toolName, version })
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        // Uninstall successful - update UI
        statusDiv.className = 'tool-status status-not-installed';
        statusDiv.textContent = t('notInstalled');

        // Hide uninstall button, show install button
        uninstallBtn.style.display = 'none';
        const installBtn = card.querySelector('.install-btn');
        installBtn.style.display = 'block';
        installBtn.disabled = false;
        pauseBtn.style.display = 'none';
        resumeBtn.style.display = 'none';
    } catch (error) {
        errorMessage.textContent = t('error') + ': ' + error.message;
        errorMessage.style.display = 'block';
        uninstallBtn.disabled = false;
        statusDiv.className = 'tool-status status-installed';
        statusDiv.textContent = t('uninstallFailed');
    }
}

// Update progress from SSE
function updateProgress(progress) {
    const cardId = 'tool-' + progress.toolName + '-' + progress.version;
    const card = document.getElementById(cardId);
    if (!card) return;

    const progressContainer = card.querySelector('.progress-container');
    const progressFill = card.querySelector('.progress-fill');
    const progressText = card.querySelector('.progress-text');
    const installBtn = card.querySelector('.install-btn');
    const pauseBtn = card.querySelector('.pause-btn');
    const resumeBtn = card.querySelector('.resume-btn');
    const uninstallBtn = card.querySelector('.uninstall-btn');
    const statusDiv = card.querySelector('.tool-status');
    const errorMessage = card.querySelector('.error-message');

    switch (progress.status) {
        case 'downloading':
            progressContainer.style.display = 'block';
            const percent = progress.totalBytes > 0
                ? (progress.downloadedBytes / progress.totalBytes * 100).toFixed(1)
                : 0;
            progressFill.style.width = percent + '%';
            const speedMB = (progress.speed / 1024 / 1024).toFixed(2);
            progressText.textContent = t('downloading') + ': ' + percent + '% (' + speedMB + ' MB/s)';
            installBtn.style.display = 'none';
            resumeBtn.style.display = 'none';
            pauseBtn.style.display = 'inline-block';
            break;

        case 'extracting':
            progressFill.style.width = '100%';
            progressText.textContent = t('extracting');
            break;

        case 'completed':
            progressContainer.style.display = 'none';
            // Hide install button, show uninstall button
            installBtn.style.display = 'none';
            uninstallBtn.style.display = 'block';
            uninstallBtn.disabled = false;
            statusDiv.className = 'tool-status status-installed';
            statusDiv.textContent = t('installed');
            pauseBtn.style.display = 'none';
            resumeBtn.style.display = 'none';
            // 安装完成后，清除信息缓存，重置信息面板与按钮
            const infoBtnC = card.querySelector('.info-btn');
            const infoPanelC = card.querySelector('.info-panel');
            if (infoBtnC && infoPanelC) {
                card.dataset.infoLoaded = 'false';
                infoPanelC.textContent = '';
                infoPanelC.style.display = 'none';
                infoBtnC.textContent = t('showInfo');
                infoBtnC.classList.remove('active');
            }
            break;

        case 'failed':
            progressContainer.style.display = 'none';
            installBtn.disabled = false;
            statusDiv.className = 'tool-status status-not-installed';
            statusDiv.textContent = t('failed');
            errorMessage.textContent = t('error') + ': ' + (progress.error || 'Unknown error');
            errorMessage.style.display = 'block';
            pauseBtn.style.display = 'none';
            resumeBtn.style.display = 'none';
            break;
        case 'paused':
            progressContainer.style.display = 'block';
            // 维持进度条显示（若后端携带总量与已下载，使用其计算百分比）
            if (typeof progress.downloadedBytes === 'number' && typeof progress.totalBytes === 'number' && progress.totalBytes > 0) {
                const ptotal = (progress.downloadedBytes / progress.totalBytes * 100).toFixed(1);
                progressFill.style.width = ptotal + '%';
                progressText.textContent = t('downloading') + ': ' + ptotal + '%';
            }
            installBtn.style.display = 'none';
            pauseBtn.style.display = 'none';
            resumeBtn.style.display = 'inline-block';
            statusDiv.className = 'tool-status status-installing';
            statusDiv.textContent = t('downloading');
            break;
        case 'uninstalled':
            // 卸载完成（来自后端主动通知）
            progressContainer.style.display = 'none';
            uninstallBtn.style.display = 'none';
            installBtn.style.display = 'block';
            installBtn.disabled = false;
            statusDiv.className = 'tool-status status-not-installed';
            statusDiv.textContent = t('notInstalled');
            pauseBtn.style.display = 'none';
            resumeBtn.style.display = 'none';
            // 卸载后同样清理信息缓存与面板
            {
                const infoBtnC = card.querySelector('.info-btn');
                const infoPanelC = card.querySelector('.info-panel');
                if (infoBtnC && infoPanelC) {
                    card.dataset.infoLoaded = 'false';
                    infoPanelC.textContent = '';
                    infoPanelC.style.display = 'none';
                    infoBtnC.textContent = t('showInfo');
                    infoBtnC.classList.remove('active');
                }
            }
            break;
    }
}

// Pause current download
async function pauseDownload(toolName, version) {
    const cardId = 'tool-' + toolName + '-' + version;
    const card = document.getElementById(cardId);
    const pauseBtn = card.querySelector('.pause-btn');
    const resumeBtn = card.querySelector('.resume-btn');
    pauseBtn.disabled = true;
    try {
        const response = await fetch('api/pause', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ toolName, version })
        });
        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }
        // 暂停成功后切换按钮
        pauseBtn.style.display = 'none';
        resumeBtn.style.display = 'inline-block';
    } catch (e) {
        console.error(e);
        pauseBtn.disabled = false;
    }
}

// Initialize on page load
window.onload = function () {
    loadTools();
    // 访问首页时先询问后端是否需要建立 SSE
    (async function () {
        try {
            const resp = await fetch('api/active');
            const data = await resp.json();
            if (data.needsSSE) {
                ensureSSE();
            } else {
                startPollingActive();
            }
            // 拉取运行时状态，恢复下载/暂停进度
            const statusResp = await fetch('api/status');
            const statuses = await statusResp.json();
            if (Array.isArray(statuses)) {
                statuses.forEach(function (s) {
                    const progress = {
                        toolName: s.name,
                        version: s.version,
                        status: s.paused ? 'paused' : (s.downloading ? 'downloading' : (s.installed ? 'completed' : '')),
                        downloadedBytes: s.downloadedBytes,
                        totalBytes: s.totalBytes,
                        speed: 0
                    };
                    if (progress.status) {
                        updateProgress(progress);
                    }
                });
            }
        } catch (e) {
            // 请求失败则开启轮询以便后续重试
            startPollingActive();
        }
    })();
};

// Cleanup on page unload
window.onbeforeunload = function () {
    closeSSE();
    stopPollingActive();
};
