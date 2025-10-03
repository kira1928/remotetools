let eventSource = null;
window.toolsData = [];

// Initialize SSE connection
function initSSE() {
    eventSource = new EventSource('/api/progress');
    
    eventSource.onmessage = function(event) {
        const progress = JSON.parse(event.data);
        updateProgress(progress);
    };

    eventSource.onerror = function() {
        console.error('SSE connection error');
    };
}

// Load tools list
async function loadTools() {
    try {
        const response = await fetch('/api/tools');
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
    const statusClass = tool.installed ? 'status-installed' : 'status-not-installed';
    const statusText = tool.installed ? t('installed') : t('notInstalled');
    statusEl.className = 'tool-status ' + statusClass;
    statusEl.textContent = statusText;
    
    // Set button - only show for non-installed tools
    const btnEl = clone.querySelector('.install-btn');
    if (tool.installed) {
        // Hide button for installed tools
        btnEl.style.display = 'none';
    } else {
        btnEl.textContent = t('install');
        btnEl.addEventListener('click', function() {
            installTool(tool.name, tool.version);
        });
    }
    
    return clone;
}

// Install tool
async function installTool(toolName, version) {
    const cardId = 'tool-' + toolName + '-' + version;
    const card = document.getElementById(cardId);
    const btn = card.querySelector('.install-btn');
    const progressContainer = card.querySelector('.progress-container');
    const errorMessage = card.querySelector('.error-message');
    const statusDiv = card.querySelector('.tool-status');

    // Reset UI
    btn.disabled = true;
    progressContainer.style.display = 'block';
    errorMessage.style.display = 'none';
    statusDiv.className = 'tool-status status-installing';
    statusDiv.textContent = t('installing');

    try {
        const response = await fetch('/api/install', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ toolName, version })
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }
    } catch (error) {
        errorMessage.textContent = t('error') + ': ' + error.message;
        errorMessage.style.display = 'block';
        btn.disabled = false;
        progressContainer.style.display = 'none';
        statusDiv.className = 'tool-status status-not-installed';
        statusDiv.textContent = t('failed');
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
    const btn = card.querySelector('.install-btn');
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
            break;

        case 'extracting':
            progressFill.style.width = '100%';
            progressText.textContent = t('extracting');
            break;

        case 'completed':
            progressContainer.style.display = 'none';
            // Hide button after successful installation
            btn.style.display = 'none';
            statusDiv.className = 'tool-status status-installed';
            statusDiv.textContent = t('installed');
            break;

        case 'failed':
            progressContainer.style.display = 'none';
            btn.disabled = false;
            statusDiv.className = 'tool-status status-not-installed';
            statusDiv.textContent = t('failed');
            errorMessage.textContent = t('error') + ': ' + (progress.error || 'Unknown error');
            errorMessage.style.display = 'block';
            break;
    }
}

// Initialize on page load
window.onload = function() {
    initSSE();
    loadTools();
};

// Cleanup on page unload
window.onbeforeunload = function() {
    if (eventSource) {
        eventSource.close();
    }
};
