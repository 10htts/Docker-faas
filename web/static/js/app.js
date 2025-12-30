// Docker FaaS Web UI Application
class DockerFaaSApp {
    constructor() {
        this.gatewayUrl = '';
        this.username = '';
        this.password = '';
        this.authenticated = false;
        this.functions = [];
        this.secrets = [];
        this.currentFunction = null;
        this.refreshInterval = null;
        this.defaultNetworkPrefix = 'docker-faas-net';
        this.networkAuto = true;
        this.serviceTouched = false;
        this.isSettingService = false;
        this.sourceFiles = [];
        this.selectedSourceFile = null;
        this.sourceLoaded = false;
        this.sourceRemovedPaths = new Set();
        this.sourceKey = '';
        this.buildHistory = [];
        this.buildHistoryLimit = 50;
        this.importPlan = null;
        this.currentBuildId = null;
        this.token = '';
        this.tokenExpiresAt = '';
        this.buildStreamAbort = null;
        this.buildStreamBuffer = '';
        this.defaultGatewayUrl = this.getDefaultGatewayUrl();
        this.loadingCount = 0;
        this.sessionTimeoutMs = 30 * 60 * 1000;
        this.inactivityTimer = null;
        this.activityHandler = null;
        this.activityEvents = ['click', 'keypress', 'mousemove', 'scroll', 'touchstart'];

        this.init();
    }

    init() {
        this.bindEvents();
        this.prefillGatewayUrl();
        this.checkSession();
        this.loadBuildHistory();
        this.resetLoadingState();
        window.addEventListener('pageshow', () => this.resetLoadingState());
    }

    bindEvents() {
        // Login
        document.getElementById('login-btn').addEventListener('click', () => this.login());
        document.getElementById('password').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.login();
        });

        // Logout
        document.getElementById('logout-btn').addEventListener('click', () => this.logout());

        // Navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', (e) => this.switchView(e.currentTarget.dataset.view));
        });

        // Overview quick actions
        document.getElementById('deploy-new-btn')?.addEventListener('click', () => this.showCreateFunction());
        document.getElementById('deploy-source-btn')?.addEventListener('click', () => this.showCreateFunction('source'));
        document.getElementById('view-functions-btn')?.addEventListener('click', () => this.switchView('functions'));
        document.getElementById('manage-secrets-btn')?.addEventListener('click', () => this.switchView('secrets'));
        document.getElementById('export-functions-btn')?.addEventListener('click', () => this.exportFunctions());
        document.getElementById('import-functions-btn')?.addEventListener('click', () => this.triggerImport());
        document.getElementById('import-functions-file')?.addEventListener('change', (e) => this.handleImportFile(e.target.files[0]));
        document.getElementById('import-confirm-btn')?.addEventListener('click', () => this.confirmImport());
        document.getElementById('import-cancel-btn')?.addEventListener('click', () => this.hideImportModal());
        document.getElementById('close-import-modal')?.addEventListener('click', () => this.hideImportModal());

        // Functions
        document.getElementById('create-function-btn')?.addEventListener('click', () => this.showCreateFunction());
        document.getElementById('function-search')?.addEventListener('input', (e) => this.filterFunctions(e.target.value));
        document.getElementById('back-to-functions')?.addEventListener('click', () => this.switchView('functions'));
        document.getElementById('form-service')?.addEventListener('input', (e) => this.handleServiceInput(e.target.value));
        document.getElementById('form-network-auto')?.addEventListener('change', (e) => this.toggleNetworkAuto(e.target.checked));

        // Function form
        document.getElementById('function-form')?.addEventListener('submit', (e) => this.submitFunctionForm(e));
        document.getElementById('cancel-function-form')?.addEventListener('click', () => this.switchView('functions'));
        document.getElementById('cancel-form-btn')?.addEventListener('click', () => this.switchView('functions'));
        document.getElementById('form-debug')?.addEventListener('change', (e) => this.toggleDebugWarning(e.target.checked));
        document.getElementById('form-deploy-mode')?.addEventListener('change', (e) => this.setDeployMode(e.target.value));
        document.getElementById('form-source-type')?.addEventListener('change', (e) => this.setSourceType(e.target.value));
        document.getElementById('form-source-git-url')?.addEventListener('input', (e) => this.handleGitUrlInput(e.target.value));
        document.getElementById('form-source-zip')?.addEventListener('change', (e) => this.handleZipInput(e.target.files[0]));
        document.getElementById('form-source-runtime')?.addEventListener('change', () => this.updatePayloadPreview());
        document.getElementById('form-source-git-ref')?.addEventListener('input', () => this.updatePayloadPreview());
        document.getElementById('form-source-path')?.addEventListener('input', () => this.updatePayloadPreview());
        document.getElementById('form-source-manifest')?.addEventListener('input', () => this.updatePayloadPreview());
        document.getElementById('source-load-btn')?.addEventListener('click', () => this.loadSourceDetails());
        document.getElementById('source-file-add')?.addEventListener('click', () => this.addOrSelectSourceFile());
        document.getElementById('source-files-refresh')?.addEventListener('click', () => this.refreshSourceFilesList());
        document.getElementById('source-file-remove')?.addEventListener('click', () => this.removeSourceFile());
        document.getElementById('source-file-content')?.addEventListener('input', (e) => this.updateSelectedSourceFile(e.target.value));
        document.getElementById('source-file-path')?.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                this.addOrSelectSourceFile();
            }
        });
        document.getElementById('source-file-list')?.addEventListener('click', (e) => {
            const target = e.target.closest('.source-file-item');
            if (target) {
                this.selectSourceFile(target.dataset.path);
            }
        });

        // Function detail
        document.getElementById('edit-function-btn')?.addEventListener('click', () => this.editFunction());
        document.getElementById('delete-function-btn')?.addEventListener('click', () => this.deleteFunction());
        document.getElementById('scale-btn')?.addEventListener('click', () => this.scaleFunction());
        document.getElementById('invoke-btn')?.addEventListener('click', () => this.invokeFunction());

        // Secrets
        document.getElementById('create-secret-btn')?.addEventListener('click', () => this.showSecretModal());
        document.getElementById('close-secret-modal')?.addEventListener('click', () => this.hideSecretModal());
        document.getElementById('cancel-secret-btn')?.addEventListener('click', () => this.hideSecretModal());
        document.getElementById('save-secret-btn')?.addEventListener('click', () => this.saveSecret());

        // Logs
        document.getElementById('fetch-logs-btn')?.addEventListener('click', () => this.fetchLogs());

        // Refresh
        document.getElementById('refresh-btn')?.addEventListener('click', () => this.refreshCurrentView());

        // Builds and metrics
        document.getElementById('clear-build-history-btn')?.addEventListener('click', () => this.clearBuildHistory());
        document.getElementById('refresh-metrics-btn')?.addEventListener('click', () => this.loadMetrics());
        document.getElementById('refresh-settings-btn')?.addEventListener('click', () => this.loadSettings());
    }

    getDefaultGatewayUrl() {
        if (window.location && window.location.origin && window.location.origin !== 'null') {
            return window.location.origin;
        }
        return 'http://localhost:8080';
    }

    normalizeGatewayUrl(url) {
        return url.replace(/\/+$/, '');
    }

    prefillGatewayUrl() {
        const input = document.getElementById('gateway-url');
        if (input && !input.value) {
            input.value = this.defaultGatewayUrl;
        }
    }

    // Session Management
    checkSession() {
        const session = localStorage.getItem('dockerfaas-session');
        if (session) {
            let data;
            try {
                data = JSON.parse(session);
            } catch (error) {
                localStorage.removeItem('dockerfaas-session');
                return;
            }
            this.gatewayUrl = this.normalizeGatewayUrl(data.gatewayUrl || this.defaultGatewayUrl);
            this.username = data.username || '';
            this.token = data.token || '';
            this.tokenExpiresAt = data.tokenExpiresAt || '';

            const gatewayInput = document.getElementById('gateway-url');
            if (gatewayInput) {
                gatewayInput.value = this.gatewayUrl;
            }
            const usernameInput = document.getElementById('username');
            if (usernameInput) {
                usernameInput.value = this.username;
            }

            if (this.token && !this.isTokenExpired(this.tokenExpiresAt)) {
                this.authenticated = true;
                this.showApp();
                this.loadOverview();
                this.refreshBuildHistory();
            } else if (this.token) {
                this.token = '';
                this.tokenExpiresAt = '';
                this.saveSession();
            }
        }
    }

    async login() {
        const gatewayInput = document.getElementById('gateway-url').value.trim();
        this.gatewayUrl = this.normalizeGatewayUrl(gatewayInput || this.defaultGatewayUrl);
        this.username = document.getElementById('username').value.trim();
        this.password = document.getElementById('password').value;

        if (!this.gatewayUrl || !this.username || !this.password) {
            this.showError('login-error', 'Please fill in all fields');
            return;
        }

        try {
            const response = await this.api('/auth/login', {
                method: 'POST',
                body: JSON.stringify({ username: this.username, password: this.password }),
                skipAuth: true
            });
            if (!response.ok) {
                const errorText = await response.text();
                this.showError('login-error', errorText || 'Authentication failed');
                return;
            }

            const data = await response.json();
            this.token = data.token || '';
            this.tokenExpiresAt = data.expiresAt || '';
            if (!this.token) {
                this.showError('login-error', 'Authentication failed');
                return;
            }
            this.authenticated = true;
            this.saveSession();

            const passwordInput = document.getElementById('password');
            if (passwordInput) {
                passwordInput.value = '';
            }
            this.password = '';
            this.showApp();
            this.loadOverview();
            this.refreshBuildHistory();
            this.startBuildStream();
            this.showToast('Connected successfully', 'success');
        } catch (error) {
            this.showError('login-error', `Connection failed: ${error.message}`);
        }
    }

    logout(options = {}) {
        const silent = options.silent === true;
        this.authenticated = false;
        this.password = '';
        const token = this.token;
        if (token) {
            this.api('/auth/logout', {
                method: 'POST',
                headers: { Authorization: `Bearer ${token}` },
                skipAuth: true,
                showLoading: false
            }).catch(() => {});
        }
        this.token = '';
        this.tokenExpiresAt = '';
        this.saveSession();
        this.hideApp();
        if (!silent) {
            this.showToast('Disconnected', 'success');
        }
    }

    showApp() {
        document.getElementById('login-screen').classList.remove('active');
        document.getElementById('app-screen').classList.add('active');
        document.getElementById('header-gateway-url').textContent = this.gatewayUrl;
        this.startAutoRefresh();
        this.setupSessionTimeout();
        this.startBuildStream();
    }

    hideApp() {
        document.getElementById('app-screen').classList.remove('active');
        document.getElementById('login-screen').classList.add('active');
        this.stopAutoRefresh();
        this.clearSessionTimeout();
        this.stopBuildStream();
        this.resetLoadingState();
    }

    // API Methods
    async api(endpoint, options = {}) {
        if (this.authenticated) {
            this.resetInactivityTimer();
        }
        const url = `${this.gatewayUrl}${endpoint}`;
        const { raw, showLoading, skipAuth, ...fetchOptions } = options;
        const headers = {
            ...fetchOptions.headers
        };

        if (!skipAuth) {
            if (this.token) {
                headers['Authorization'] = `Bearer ${this.token}`;
            } else if (this.username && this.password) {
                headers['Authorization'] = 'Basic ' + btoa(`${this.username}:${this.password}`);
            }
        }

        const hasContentType = Object.keys(headers).some((key) => key.toLowerCase() === 'content-type');
        if (!raw && fetchOptions.body !== undefined && !hasContentType) {
            headers['Content-Type'] = 'application/json';
        }

        const method = (fetchOptions.method || 'GET').toUpperCase();
        const shouldShowLoading = showLoading === true || (showLoading !== false && method !== 'GET');
        if (shouldShowLoading) {
            this.showLoading();
        }

        try {
            const response = await fetch(url, {
                ...fetchOptions,
                headers
            });
            if (this.authenticated && (response.status === 401 || response.status === 403)) {
                this.showToast('Session expired. Please log in again.', 'warning');
                this.logout({ silent: true });
            }
            return response;
        } finally {
            if (shouldShowLoading) {
                this.hideLoading();
            }
        }
    }

    // View Management
    switchView(viewName) {
        // Update navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.remove('active');
            if (item.dataset.view === viewName) {
                item.classList.add('active');
            }
        });

        // Update views
        document.querySelectorAll('.view').forEach(view => {
            view.classList.remove('active');
        });
        document.getElementById(`${viewName}-view`)?.classList.add('active');

        // Load data for view
        switch (viewName) {
            case 'overview':
                this.loadOverview();
                break;
            case 'functions':
                this.loadFunctions();
                break;
            case 'builds':
                this.loadBuildHistoryView();
                break;
            case 'secrets':
                this.loadSecrets();
                break;
            case 'logs':
                this.loadLogsView();
                break;
            case 'metrics':
                this.loadMetrics();
                break;
            case 'settings':
                this.loadSettings();
                break;
        }
    }

    refreshCurrentView() {
        const activeView = document.querySelector('.view.active');
        if (activeView) {
            const viewId = activeView.id;
            if (viewId === 'function-form-view' || viewId === 'function-detail-view') {
                return;
            }
        }

        const activeNav = document.querySelector('.nav-item.active');
        if (activeNav) {
            this.switchView(activeNav.dataset.view);
        }
    }

    startAutoRefresh() {
        this.refreshInterval = setInterval(() => {
            this.refreshCurrentView();
        }, 30000); // Refresh every 30 seconds
    }

    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    saveSession() {
        localStorage.setItem('dockerfaas-session', JSON.stringify({
            gatewayUrl: this.gatewayUrl,
            username: this.username,
            token: this.token,
            tokenExpiresAt: this.tokenExpiresAt
        }));
    }

    setupSessionTimeout() {
        this.clearSessionTimeout();
        this.activityHandler = () => this.resetInactivityTimer();
        this.activityEvents.forEach((eventName) => {
            document.addEventListener(eventName, this.activityHandler, { passive: true });
        });
        this.resetInactivityTimer();
    }

    resetInactivityTimer() {
        if (!this.authenticated) {
            return;
        }
        if (this.inactivityTimer) {
            clearTimeout(this.inactivityTimer);
        }
        let timeoutMs = this.sessionTimeoutMs;
        if (this.tokenExpiresAt) {
            const expiresAt = new Date(this.tokenExpiresAt).getTime();
            if (!Number.isNaN(expiresAt)) {
                const remaining = expiresAt - Date.now();
                if (remaining > 0 && remaining < timeoutMs) {
                    timeoutMs = remaining;
                }
            }
        }
        this.inactivityTimer = setTimeout(() => {
            this.showToast('Session expired. Please log in again.', 'warning');
            this.logout({ silent: true });
        }, timeoutMs);
    }

    clearSessionTimeout() {
        if (this.inactivityTimer) {
            clearTimeout(this.inactivityTimer);
            this.inactivityTimer = null;
        }
        if (this.activityHandler) {
            this.activityEvents.forEach((eventName) => {
                document.removeEventListener(eventName, this.activityHandler);
            });
            this.activityHandler = null;
        }
    }

    // Build History
    loadBuildHistory() {
        this.buildHistory = [];
    }

    saveBuildHistory() {
    }

    addBuildHistory(entry) {
        this.buildHistory.unshift(entry);
        if (this.buildHistory.length > this.buildHistoryLimit) {
            this.buildHistory = this.buildHistory.slice(0, this.buildHistoryLimit);
        }
        this.renderBuildsTable();
    }

    updateBuildHistory(id, updates) {
        const index = this.buildHistory.findIndex((entry) => entry.id === id);
        if (index === -1) {
            return;
        }
        this.buildHistory[index] = { ...this.buildHistory[index], ...updates };
        this.renderBuildsTable();
        if (this.currentBuildId === id) {
            this.renderBuildDetails(this.buildHistory[index]);
        }
    }

    clearBuildHistory() {
        if (!confirm('Clear build history?')) {
            return;
        }
        if (!this.authenticated) {
            this.buildHistory = [];
            this.renderBuildsTable();
            this.hideBuildDetails();
            return;
        }
        this.api('/system/builds', { method: 'DELETE' })
            .then(() => {
                this.buildHistory = [];
                this.renderBuildsTable();
                this.hideBuildDetails();
                this.showToast('Build history cleared', 'success');
            })
            .catch((error) => {
                this.showToast(`Failed to clear history: ${error.message}`, 'error');
            });
    }

    async refreshBuildHistory() {
        if (!this.authenticated) {
            return;
        }
        try {
            const limit = Number.isFinite(this.buildHistoryLimit) && this.buildHistoryLimit > 0 ? this.buildHistoryLimit : 50;
            const response = await this.api(`/system/builds?limit=${limit}&includeOutput=true`, { showLoading: false });
            if (!response.ok) {
                return;
            }
            const entries = await response.json();
            if (Array.isArray(entries)) {
                this.buildHistory = entries;
                this.renderBuildsTable();
                if (this.currentBuildId) {
                    const entry = this.buildHistory.find((item) => item.id === this.currentBuildId);
                    if (entry) {
                        this.renderBuildDetails(entry);
                    }
                }
            }
        } catch (error) {
            this.showToast(`Failed to load builds: ${error.message}`, 'error');
        }
    }

    applyBuildUpdate(entry) {
        if (!entry || !entry.id) {
            return;
        }
        const index = this.buildHistory.findIndex((item) => item.id === entry.id);
        if (index === -1) {
            this.buildHistory.unshift(entry);
        } else {
            this.buildHistory[index] = entry;
        }
        if (this.buildHistory.length > this.buildHistoryLimit) {
            this.buildHistory = this.buildHistory.slice(0, this.buildHistoryLimit);
        }
        this.renderBuildsTable();
        if (this.currentBuildId === entry.id) {
            this.renderBuildDetails(entry);
        }
    }

    startBuildStream() {
        if (!this.authenticated || this.buildStreamAbort) {
            return;
        }
        this.buildStreamAbort = new AbortController();
        this.consumeBuildStream(this.buildStreamAbort.signal);
    }

    stopBuildStream() {
        if (this.buildStreamAbort) {
            this.buildStreamAbort.abort();
            this.buildStreamAbort = null;
        }
        this.buildStreamBuffer = '';
    }

    async consumeBuildStream(signal) {
        while (this.authenticated && !signal.aborted) {
            try {
                const response = await this.api('/system/builds/stream', {
                    showLoading: false,
                    raw: true,
                    signal
                });
                if (!this.authenticated || signal.aborted) {
                    return;
                }
                if (!response.ok || !response.body) {
                    await this.sleep(3000);
                    continue;
                }

                const reader = response.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';

                while (!signal.aborted) {
                    const { value, done } = await reader.read();
                    if (done) {
                        break;
                    }
                    buffer += decoder.decode(value, { stream: true });
                    const parts = buffer.split('\n\n');
                    buffer = parts.pop() || '';
                    for (const part of parts) {
                        const lines = part.split('\n');
                        for (const line of lines) {
                            if (line.startsWith('data:')) {
                                const payload = line.replace(/^data:\s*/, '');
                                try {
                                    const entry = JSON.parse(payload);
                                    this.applyBuildUpdate(entry);
                                } catch (error) {
                                    continue;
                                }
                            }
                        }
                    }
                }
            } catch (error) {
                if (!signal.aborted) {
                    await this.sleep(3000);
                }
            }
        }
    }

    sleep(ms) {
        return new Promise((resolve) => setTimeout(resolve, ms));
    }

    // Overview
    // Overview
    async loadOverview() {
        try {
            const infoResponse = await this.api('/system/info');
            if (infoResponse.ok) {
                const info = await infoResponse.json();
                const version = info.version?.release || info.version?.sha || info.provider?.version || 'v2.0';
                document.getElementById('stat-version').textContent = version;
            }

            const health = await this.fetchHealthChecks();
            if (health) {
                if (health.status === 'ok') {
                    this.setHealthStatus('healthy', 'Healthy');
                } else {
                    this.setHealthStatus('error', 'Unhealthy');
                }
            } else if (!infoResponse.ok) {
                this.setHealthStatus('error', 'Error');
            }

            // Load functions for stats
            const functions = await this.fetchFunctions();
            const totalReplicas = functions.reduce((sum, fn) => sum + (fn.replicas || 0), 0);
            const debugEnabled = functions.filter(fn => fn.debug).length;

            document.getElementById('stat-functions').textContent = functions.length;
            document.getElementById('stat-replicas').textContent = totalReplicas;
            document.getElementById('stat-debug').textContent = debugEnabled;
        } catch (error) {
            this.setHealthStatus('error', 'Connection Error');
            this.showToast(`Error loading overview: ${error.message}`, 'error');
        }
    }

    setHealthStatus(status, text) {
        const indicator = document.getElementById('health-status');
        indicator.className = 'health-indicator ' + status;
        indicator.querySelector('.status-text').textContent = text;
    }

    // Functions
    async fetchFunctions() {
        const response = await this.api('/system/functions');
        if (!response.ok) {
            throw new Error('Failed to load functions');
        }

        const functions = await response.json();
        this.functions = functions;
        return functions;
    }

    async loadFunctions() {
        try {
            await this.fetchFunctions();
            this.renderFunctionsTable();
        } catch (error) {
            this.showToast(`Error loading functions: ${error.message}`, 'error');
        }
    }

    renderFunctionsTable(filter = '') {
        const tbody = document.getElementById('functions-tbody');
        const filteredFunctions = this.functions.filter(fn =>
            fn.name.toLowerCase().includes(filter.toLowerCase()) ||
            fn.image.toLowerCase().includes(filter.toLowerCase())
        );

        if (filteredFunctions.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="empty-state">No functions found</td></tr>';
            return;
        }

        tbody.innerHTML = filteredFunctions.map(fn => `
            <tr>
                <td><strong>${fn.name}</strong></td>
                <td><code>${fn.image}</code></td>
                <td>${fn.replicas || 0}</td>
                <td><code>${fn.network || '-'}</code></td>
                <td>${fn.debug ? '<span class="badge badge-warning">Yes</span>' : '<span class="badge badge-info">No</span>'}</td>
                <td>${this.formatDate(fn.updatedAt || fn.createdAt) || '-'}</td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="app.viewFunction('${fn.name}')">View</button>
                </td>
            </tr>
        `).join('');
    }

    filterFunctions(query) {
        this.renderFunctionsTable(query);
    }

    async viewFunction(name) {
        try {
            const functions = await this.fetchFunctions();
            this.currentFunction = functions.find(f => f.name === name);

            if (this.currentFunction) {
                this.renderFunctionDetail();
                document.getElementById('function-detail-view').classList.add('active');
                document.getElementById('functions-view').classList.remove('active');
            } else {
                this.showToast('Function not found', 'error');
            }
        } catch (error) {
            this.showToast(`Error loading function: ${error.message}`, 'error');
        }
    }

    renderFunctionDetail() {
        const fn = this.currentFunction;
        document.getElementById('function-detail-name').textContent = fn.name;

        const envKeys = this.formatKeyList(fn.envVars);
        const labelKeys = this.formatKeyList(fn.labels);
        const limits = this.formatResources(fn.limits);
        const requests = this.formatResources(fn.requests);

        const config = document.getElementById('function-config');
        config.innerHTML = `
            <dt>Image</dt>
            <dd>${fn.image}</dd>
            <dt>Network</dt>
            <dd>${fn.network || '-'}</dd>
            <dt>Replicas (Desired)</dt>
            <dd>${fn.replicas || 0}</dd>
            <dt>Replicas (Available)</dt>
            <dd>${fn.availableReplicas || 0}</dd>
            <dt>Debug Mode</dt>
            <dd>${fn.debug ? '<span class="text-warning">Enabled</span>' : 'Disabled'}</dd>
            <dt>Read-only FS</dt>
            <dd>${fn.readOnlyRootFilesystem ? 'Yes' : 'No'}</dd>
            ${limits ? `<dt>Limits</dt><dd>${limits}</dd>` : ''}
            ${requests ? `<dt>Requests</dt><dd>${requests}</dd>` : ''}
            ${envKeys ? `<dt>Env Vars</dt><dd>${envKeys}</dd>` : ''}
            ${labelKeys ? `<dt>Labels</dt><dd>${labelKeys}</dd>` : ''}
            ${fn.secrets && fn.secrets.length > 0 ? `<dt>Secrets</dt><dd>${fn.secrets.join(', ')}</dd>` : ''}
        `;

        document.getElementById('scale-input').value = fn.replicas || 1;

        // Load and render replicas
        this.loadReplicas(fn.name);
    }

    async loadReplicas(functionName) {
        try {
            const response = await this.api(`/system/function/${encodeURIComponent(functionName)}/containers`);
            if (response.ok) {
                const containers = await response.json();
                this.renderReplicas(containers);
            } else {
                document.getElementById('replicas-list').innerHTML = '<p class="text-muted">No replica information available</p>';
            }
        } catch (error) {
            console.error('Error loading replicas:', error);
        }
    }

    renderReplicas(replicas) {
        const container = document.getElementById('replicas-list');
        if (!replicas || replicas.length === 0) {
            container.innerHTML = '<p class="text-muted">No replicas</p>';
            return;
        }

        container.innerHTML = replicas.map(replica => `
            <div class="replica-card">
                <div class="replica-header">
                    <span class="replica-name">${replica.name || 'Replica'}</span>
                    <span class="badge ${this.isReplicaHealthy(replica.status) ? 'badge-success' : 'badge-warning'}">
                        ${replica.status || 'Unknown'}
                    </span>
                </div>
                <div class="replica-details">
                    ${replica.ipAddress ? `IP: ${replica.ipAddress}` : 'IP: -'}
                    ${replica.ports && Object.keys(replica.ports).length > 0 ? `<br>Ports: ${Object.entries(replica.ports).map(([containerPort, hostPort]) => `${containerPort} -> ${hostPort}`).join(', ')}` : '<br>Ports: -'}
                    ${replica.createdAt ? `<br>Created: ${this.formatDate(replica.createdAt)}` : ''}
                </div>
            </div>
        `).join('');
    }

    async scaleFunction() {
        const replicas = parseInt(document.getElementById('scale-input').value);
        if (isNaN(replicas) || replicas < 0) {
            this.showToast('Invalid replica count', 'error');
            return;
        }

        try {
            const response = await this.api(`/system/scale-function/${encodeURIComponent(this.currentFunction.name)}`, {
                method: 'POST',
                body: JSON.stringify({ serviceName: this.currentFunction.name, replicas })
            });

            if (response.ok) {
                this.showToast('Function scaled successfully', 'success');
                this.viewFunction(this.currentFunction.name);
            } else {
                const error = await response.text();
                this.showToast(`Failed to scale function: ${error}`, 'error');
            }
        } catch (error) {
            this.showToast(`Error scaling function: ${error.message}`, 'error');
        }
    }

    async invokeFunction() {
        const method = document.getElementById('invoke-method').value;
        const headersInput = document.getElementById('invoke-headers').value;
        const body = document.getElementById('invoke-body').value;

        let headers = {};
        if (headersInput) {
            try {
                headers = JSON.parse(headersInput);
            } catch (e) {
                this.showToast('Invalid headers JSON', 'error');
                return;
            }
        }

        const startTime = Date.now();
        try {
            const response = await this.api(`/function/${encodeURIComponent(this.currentFunction.name)}`, {
                method,
                headers,
                body: body || undefined,
                raw: true
            });

            const latency = Date.now() - startTime;
            const responseBody = await response.text();
            const responseHeaders = {};
            response.headers.forEach((value, key) => {
                responseHeaders[key] = value;
            });

            const resultPanel = document.getElementById('invoke-response');
            resultPanel.innerHTML = `
                <strong>Status:</strong> ${response.status} ${response.statusText}<br>
                <strong>Latency:</strong> ${latency}ms<br>
                <strong>Headers:</strong><br>
                <pre>${JSON.stringify(responseHeaders, null, 2)}</pre>
                <strong>Body:</strong><br>
                <pre>${responseBody}</pre>
            `;
        } catch (error) {
            document.getElementById('invoke-response').innerHTML = `
                <span class="text-danger">Error: ${error.message}</span>
            `;
        }
    }

    async deleteFunction() {
        if (!confirm(`Are you sure you want to delete function "${this.currentFunction.name}"?`)) {
            return;
        }

        try {
            const response = await this.api(`/system/functions?functionName=${encodeURIComponent(this.currentFunction.name)}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                this.showToast('Function deleted successfully', 'success');
                this.switchView('functions');
            } else {
                const error = await response.text();
                this.showToast(`Failed to delete function: ${error}`, 'error');
            }
        } catch (error) {
            this.showToast(`Error deleting function: ${error.message}`, 'error');
        }
    }

    // Function Form
    showCreateFunction(mode = 'image') {
        document.getElementById('function-form-title').textContent = 'Create Function';
        document.getElementById('function-form').reset();
        document.getElementById('form-service').disabled = false;
        document.getElementById('form-deploy-mode').disabled = false;
        this.currentFunction = null;
        this.serviceTouched = false;
        this.networkAuto = true;
        this.sourceFiles = [];
        this.selectedSourceFile = null;
        this.sourceLoaded = false;
        this.sourceRemovedPaths = new Set();
        this.sourceKey = '';
        document.getElementById('form-network-auto').checked = true;
        this.toggleNetworkAuto(true);
        document.getElementById('form-deploy-mode').value = mode;
        this.setDeployMode(mode);
        this.updateSourceEditorState();

        document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
        document.getElementById('function-form-view').classList.add('active');
    }

    editFunction() {
        document.getElementById('function-form-title').textContent = 'Edit Function';
        this.populateFunctionForm(this.currentFunction);
        document.getElementById('form-service').disabled = true;
        document.getElementById('form-deploy-mode').disabled = true;
        document.getElementById('form-deploy-mode').value = 'image';
        this.setDeployMode('image');
        this.toggleNetworkAuto(false);

        document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
        document.getElementById('function-form-view').classList.add('active');
    }

    populateFunctionForm(fn) {
        document.getElementById('form-service').value = fn.name;
        document.getElementById('form-image').value = fn.image;
        document.getElementById('form-network').value = fn.network || '';
        document.getElementById('form-env').value = fn.envVars ? JSON.stringify(fn.envVars, null, 2) : '';
        document.getElementById('form-labels').value = fn.labels ? JSON.stringify(fn.labels, null, 2) : '';
        document.getElementById('form-secrets').value = fn.secrets ? fn.secrets.join(',') : '';
        document.getElementById('form-memory').value = fn.limits?.memory || '';
        document.getElementById('form-cpu').value = fn.limits?.cpu || '';
        document.getElementById('form-readonly').checked = fn.readOnlyRootFilesystem || false;
        document.getElementById('form-debug').checked = fn.debug || false;
        this.toggleDebugWarning(fn.debug || false);
    }

    toggleDebugWarning(enabled) {
        const warning = document.getElementById('debug-warning');
        warning.style.display = enabled ? 'block' : 'none';
    }

    handleServiceInput(value) {
        if (this.isSettingService) {
            return;
        }
        this.serviceTouched = value.trim().length > 0;
        this.updateNetworkAuto();
    }

    setServiceValue(value) {
        const input = document.getElementById('form-service');
        this.isSettingService = true;
        input.value = value;
        this.isSettingService = false;
        this.serviceTouched = false;
        this.updateNetworkAuto();
    }

    getDerivedNetwork(serviceName) {
        const name = serviceName.trim();
        if (!name) {
            return '';
        }
        return `${this.defaultNetworkPrefix}-${name}`;
    }

    updateNetworkAuto() {
        if (!this.networkAuto) {
            return;
        }
        const service = document.getElementById('form-service').value;
        const networkInput = document.getElementById('form-network');
        networkInput.value = this.getDerivedNetwork(service);
    }

    toggleNetworkAuto(enabled) {
        const networkInput = document.getElementById('form-network');
        this.networkAuto = enabled;
        networkInput.disabled = enabled;
        if (enabled) {
            this.updateNetworkAuto();
        }
    }

    handleGitUrlInput(value) {
        const trimmed = value.trim();
        if (!trimmed) {
            this.updateSourceBasicVisibility();
            return;
        }
        if (this.serviceTouched) {
            return;
        }
        const repoPart = trimmed.split('/').filter(Boolean).pop() || '';
        const name = repoPart.replace(/\.git$/, '');
        if (name) {
            this.setServiceValue(name);
        }
        this.updateSourceBasicVisibility();
    }

    handleZipInput(file) {
        if (!file) {
            this.updateSourceBasicVisibility();
            return;
        }
        if (this.serviceTouched) {
            return;
        }
        const name = file.name.replace(/\.zip$/i, '');
        if (name) {
            this.setServiceValue(name);
        }
        this.updatePayloadPreview();
        this.updateSourceBasicVisibility();
    }

    setDeployMode(mode) {
        const imageSection = document.getElementById('image-config');
        const sourceSection = document.getElementById('source-config');
        const imageInput = document.getElementById('form-image');
        const submitBtn = document.getElementById('submit-function-btn');
        const sourceWarning = document.getElementById('source-warning');
        const envSection = document.getElementById('env-section');
        const labelsSection = document.getElementById('labels-section');
        const secretsSection = document.getElementById('secrets-section');
        const limitsSection = document.getElementById('limits-section');
        const advancedSection = document.getElementById('advanced-section');
        const networkAutoToggle = document.getElementById('form-network-auto');
        const basicSection = document.getElementById('basic-section');

        if (mode === 'source') {
            imageSection.style.display = 'none';
            sourceSection.style.display = 'block';
            imageInput.required = false;
            submitBtn.textContent = 'Build Function';
            sourceWarning.style.display = 'block';
            const sourceTypeSelect = document.getElementById('form-source-type');
            if (sourceTypeSelect) {
                this.setSourceType(sourceTypeSelect.value);
            }
            this.setSectionVisible(envSection, false);
            this.setSectionVisible(labelsSection, false);
            this.setSectionVisible(secretsSection, false);
            this.setSectionVisible(limitsSection, false);
            this.setSectionVisible(advancedSection, false);
            this.setSectionVisible(basicSection, false);
            if (networkAutoToggle) {
                networkAutoToggle.checked = true;
                this.toggleNetworkAuto(true);
            }
            this.sourceLoaded = false;
            this.updateSourceEditorState();
            this.updateSourceBasicVisibility();
        } else {
            imageSection.style.display = 'block';
            sourceSection.style.display = 'none';
            imageInput.required = true;
            submitBtn.textContent = 'Deploy Function';
            sourceWarning.style.display = 'none';
            this.setSectionVisible(envSection, true);
            this.setSectionVisible(labelsSection, true);
            this.setSectionVisible(secretsSection, true);
            this.setSectionVisible(limitsSection, true);
            this.setSectionVisible(advancedSection, true);
            this.setSectionVisible(basicSection, true);
            this.updateSourceEditorState();
        }
    }

    updateSourceBasicVisibility() {
        const mode = document.getElementById('form-deploy-mode').value;
        const basicSection = document.getElementById('basic-section');
        if (!basicSection) {
            return;
        }
        if (mode !== 'source') {
            basicSection.style.display = 'block';
            this.updateSourceEditorState();
            return;
        }
        basicSection.style.display = this.sourceLoaded ? 'block' : 'none';
        this.updateSourceEditorState();
    }

    setSectionVisible(section, visible) {
        if (!section) {
            return;
        }
        section.style.display = visible ? 'block' : 'none';
    }

    updateSourceEditorState() {
        const mode = document.getElementById('form-deploy-mode').value;
        const editorSection = document.getElementById('source-editor-section');
        if (!editorSection) {
            return;
        }
        editorSection.style.display = mode === 'source' && this.sourceLoaded ? 'block' : 'none';
        if (mode === 'source') {
            this.renderSourceFilesList();
            this.updatePayloadPreview();
        }
    }

    async loadSourceDetails() {
        const status = document.getElementById('source-load-status');
        const sourceType = document.getElementById('form-source-type').value;
        const zipFile = document.getElementById('form-source-zip').files[0];
        const gitUrl = document.getElementById('form-source-git-url').value.trim();
        const gitRef = document.getElementById('form-source-git-ref').value.trim();
        const sourcePath = document.getElementById('form-source-path').value.trim();
        const runtime = document.getElementById('form-source-runtime').value;
        const manifestOverride = document.getElementById('form-source-manifest').value.trim();
        const sourceKey = this.getSourceKey(sourceType, zipFile, gitUrl, gitRef, sourcePath);
        if (sourceKey && sourceKey !== this.sourceKey) {
            this.sourceFiles = [];
            this.selectedSourceFile = null;
            this.sourceRemovedPaths = new Set();
            this.sourceKey = sourceKey;
        }

        if (status) {
            status.textContent = 'Loading...';
        }

        try {
            let response;
            if (sourceType === 'zip') {
                if (!zipFile) {
                    this.showToast('Please select a zip file', 'warning');
                    if (status) status.textContent = '';
                    return;
                }

                const formData = new FormData();
                formData.append('name', document.getElementById('form-service').value.trim());
                if (runtime) {
                    formData.append('runtime', runtime);
                }
                formData.append('sourceType', sourceType);
                if (manifestOverride) {
                    formData.append('manifest', manifestOverride);
                }
                formData.append('file', zipFile);

                response = await this.api('/system/builds/inspect', {
                    method: 'POST',
                    body: formData,
                    raw: true
                });
            } else {
                if (!gitUrl) {
                    this.showToast('Please enter a Git URL', 'warning');
                    if (status) status.textContent = '';
                    return;
                }

                const payload = {
                    name: document.getElementById('form-service').value.trim(),
                    source: {
                        type: 'git',
                        git: {
                            url: gitUrl,
                            ref: gitRef || 'main',
                            path: sourcePath || '.'
                        },
                        manifest: manifestOverride || undefined
                    }
                };
                if (runtime) {
                    payload.source.runtime = runtime;
                }

                response = await this.api('/system/builds/inspect', {
                    method: 'POST',
                    body: JSON.stringify(payload)
                });
            }

            if (!response.ok) {
                const errorText = await response.text();
                this.showToast(`Inspect failed: ${errorText}`, 'error');
                if (status) status.textContent = '';
                this.sourceLoaded = false;
                this.updateSourceEditorState();
                return;
            }

            const data = await response.json();
            if (typeof data.manifest === 'string') {
                document.getElementById('form-source-manifest').value = data.manifest;
            }
            if (data.runtime) {
                document.getElementById('form-source-runtime').value = data.runtime;
            }
            if (data.name) {
                this.setServiceValue(data.name);
            }

            this.sourceLoaded = true;
            this.updateSourceEditorState();
            this.updateSourceBasicVisibility();
            this.applySourceFileList(data.files || []);

            if (status) {
                status.textContent = 'Loaded';
            }
            this.showToast('Source loaded. Review docker-faas.yaml before building.', 'success');
            this.updatePayloadPreview();
        } catch (error) {
            if (status) status.textContent = '';
            this.sourceLoaded = false;
            this.updateSourceEditorState();
            this.updateSourceBasicVisibility();
            this.showToast(`Inspect error: ${error.message}`, 'error');
        }
    }

    getSourceKey(sourceType, zipFile, gitUrl, gitRef, sourcePath) {
        if (sourceType === 'zip' && zipFile) {
            return `zip:${zipFile.name}:${zipFile.size}:${zipFile.lastModified || 0}`;
        }
        if (sourceType === 'git' && gitUrl) {
            return `git:${gitUrl}:${gitRef || 'main'}:${sourcePath || '.'}`;
        }
        return '';
    }

    addOrSelectSourceFile() {
        const pathInput = document.getElementById('source-file-path');
        const path = pathInput.value.trim();
        if (!path) {
            this.showToast('Enter a file path', 'warning');
            return;
        }

        const existing = this.sourceFiles.find((file) => file.path === path);
        if (existing) {
            this.selectSourceFile(path);
            return;
        }

        if (this.sourceRemovedPaths.has(path)) {
            this.sourceRemovedPaths.delete(path);
        }

        const file = { path, content: '', originalContent: '', editable: true, modified: true, fromSource: false };
        this.sourceFiles.push(file);
        pathInput.value = '';
        this.selectSourceFile(path);
        this.renderSourceFilesList();
        this.updatePayloadPreview();
    }

    selectSourceFile(path) {
        const file = this.sourceFiles.find((entry) => entry.path === path);
        this.selectedSourceFile = file || null;
        const contentInput = document.getElementById('source-file-content');
        const removeBtn = document.getElementById('source-file-remove');

        if (!file) {
            contentInput.value = '';
            contentInput.disabled = true;
            contentInput.readOnly = false;
            contentInput.placeholder = 'File contents...';
            removeBtn.disabled = true;
            return;
        }

        const editable = file.editable !== false;
        contentInput.value = file.content || '';
        contentInput.disabled = false;
        contentInput.readOnly = !editable;
        contentInput.placeholder = editable ? 'File contents...' : 'File is binary or too large to edit.';
        removeBtn.disabled = false;
        this.renderSourceFilesList();
    }

    updateSelectedSourceFile(content) {
        if (!this.selectedSourceFile || this.selectedSourceFile.editable === false) {
            return;
        }
        this.selectedSourceFile.content = content;
        this.selectedSourceFile.modified = true;
        const removeBtn = document.getElementById('source-file-remove');
        if (removeBtn) {
            removeBtn.disabled = false;
        }
        this.updatePayloadPreview();
    }

    removeSourceFile() {
        if (!this.selectedSourceFile) {
            return;
        }
        const path = this.selectedSourceFile.path;
        if (this.selectedSourceFile.fromSource) {
            this.sourceRemovedPaths.add(path);
        }
        this.sourceFiles = this.sourceFiles.filter((file) => file.path !== path);
        this.selectedSourceFile = null;
        document.getElementById('source-file-content').value = '';
        document.getElementById('source-file-content').disabled = true;
        document.getElementById('source-file-content').readOnly = false;
        document.getElementById('source-file-remove').disabled = true;
        this.renderSourceFilesList();
        this.updatePayloadPreview();
    }

    renderSourceFilesList() {
        const list = document.getElementById('source-file-list');
        if (!list) {
            return;
        }

        if (this.sourceFiles.length === 0) {
            list.innerHTML = '<div class="text-muted">No files loaded.</div>';
            return;
        }

        const grouped = this.groupFilesByFolder(this.sourceFiles.map((file) => file.path));
        list.innerHTML = this.renderFileTree(grouped);
    }

    groupFilesByFolder(paths) {
        const root = { files: [], dirs: {} };
        paths.forEach((path) => {
            const parts = path.split('/').filter(Boolean);
            let node = root;
            parts.forEach((part, idx) => {
                if (idx === parts.length - 1) {
                    node.files.push(parts.join('/'));
                } else {
                    node.dirs[part] = node.dirs[part] || { files: [], dirs: {} };
                    node = node.dirs[part];
                }
            });
        });
        return root;
    }

    renderFileTree(node, prefix = '') {
        const sections = [];
        const folderNames = Object.keys(node.dirs).sort();
        folderNames.forEach((folder) => {
            const nextPrefix = prefix ? `${prefix}/${folder}` : folder;
            sections.push(`
                <div class="source-folder">
                    <div class="source-folder-title">${nextPrefix}/</div>
                    ${this.renderFileTree(node.dirs[folder], nextPrefix)}
                </div>
            `);
        });

        const fileButtons = node.files
            .filter((filePath) => filePath)
            .sort()
            .map((filePath) => {
                const active = this.selectedSourceFile && this.selectedSourceFile.path === filePath ? ' active' : '';
                const label = filePath.split('/').pop();
                return `<button class="source-file-item${active}" data-path="${filePath}" type="button">${label}</button>`;
            });

        return [...sections, ...fileButtons].join('');
    }

    async refreshSourceFilesList() {
        if (!this.sourceLoaded) {
            this.showToast('Load source first', 'warning');
            return;
        }
        await this.loadSourceDetails();
    }

    applySourceFileList(entries) {
        if (!Array.isArray(entries) || entries.length === 0) {
            this.sourceFiles = [];
            this.selectedSourceFile = null;
            const contentInput = document.getElementById('source-file-content');
            const removeBtn = document.getElementById('source-file-remove');
            if (contentInput) {
                contentInput.value = '';
                contentInput.disabled = true;
                contentInput.readOnly = false;
                contentInput.placeholder = 'File contents...';
            }
            if (removeBtn) {
                removeBtn.disabled = true;
            }
            this.renderSourceFilesList();
            return;
        }
        const existing = new Map(this.sourceFiles.map((file) => [file.path, file]));
        const normalized = entries
            .map((entry) => {
                if (typeof entry === 'string') {
                    return {
                        path: entry,
                        content: '',
                        originalContent: '',
                        editable: true,
                        modified: false,
                        fromSource: true
                    };
                }
                if (entry && typeof entry === 'object' && typeof entry.path === 'string') {
                    const baseContent = typeof entry.content === 'string' ? entry.content : '';
                    return {
                        path: entry.path,
                        content: baseContent,
                        originalContent: baseContent,
                        editable: entry.editable !== false,
                        modified: false,
                        fromSource: true
                    };
                }
                return null;
            })
            .filter((entry) => entry && entry.path)
            .map((entry) => {
                const path = entry.path.replace(/\\/g, '/');
                if (this.sourceRemovedPaths.has(path)) {
                    return null;
                }
                const existingEntry = existing.get(path);
                if (existingEntry && existingEntry.modified) {
                    return {
                        ...entry,
                        content: existingEntry.content,
                        originalContent: existingEntry.originalContent || entry.originalContent || '',
                        editable: existingEntry.editable !== false,
                        modified: true,
                        fromSource: existingEntry.fromSource !== false
                    };
                }
                return { ...entry, path };
            })
            .filter(Boolean)
            .sort((a, b) => a.path.localeCompare(b.path));
        const normalizedPaths = new Set(normalized.map((file) => file.path));
        existing.forEach((entry, path) => {
            if (entry.modified && !normalizedPaths.has(path)) {
                normalized.push(entry);
            }
        });
        normalized.sort((a, b) => a.path.localeCompare(b.path));
        this.sourceFiles = normalized;
        const activePath = this.selectedSourceFile ? this.selectedSourceFile.path : '';
        this.selectedSourceFile = activePath ? this.sourceFiles.find((file) => file.path === activePath) || null : null;
        const contentInput = document.getElementById('source-file-content');
        const removeBtn = document.getElementById('source-file-remove');
        if (contentInput) {
            contentInput.value = this.selectedSourceFile ? this.selectedSourceFile.content || '' : '';
            contentInput.disabled = !this.selectedSourceFile;
            contentInput.readOnly = !!(this.selectedSourceFile && this.selectedSourceFile.editable === false);
            contentInput.placeholder = this.selectedSourceFile && this.selectedSourceFile.editable === false
                ? 'File is binary or too large to edit.'
                : 'File contents...';
        }
        if (removeBtn) {
            removeBtn.disabled = !this.selectedSourceFile;
        }
        this.renderSourceFilesList();
        this.updatePayloadPreview();
    }

    buildSourcePayload() {
        const service = document.getElementById('form-service').value.trim();
        const sourceType = document.getElementById('form-source-type').value;
        const runtime = document.getElementById('form-source-runtime').value;
        const gitUrl = document.getElementById('form-source-git-url').value.trim();
        const gitRef = document.getElementById('form-source-git-ref').value.trim();
        const sourcePath = document.getElementById('form-source-path').value.trim();
        const manifestOverride = document.getElementById('form-source-manifest').value.trim();
        const zipFile = document.getElementById('form-source-zip').files[0];

        const payload = {
            name: service || '',
            source: {
                type: sourceType
            }
        };

        if (runtime) {
            payload.source.runtime = runtime;
        }

        if (sourceType === 'git') {
            payload.source.git = {
                url: gitUrl || '',
                ref: gitRef || 'main',
                path: sourcePath || '.'
            };
        } else if (zipFile) {
            payload.source.zip = {
                filename: zipFile.name
            };
        }

        if (manifestOverride) {
            payload.source.manifest = manifestOverride;
        }

        const modifiedFiles = this.sourceFiles.filter((file) => file.modified);
        const removedFiles = this.sourceRemovedPaths.size > 0 ? Array.from(this.sourceRemovedPaths) : [];
        if (modifiedFiles.length > 0 || removedFiles.length > 0) {
            payload.source.files = [
                ...modifiedFiles.map((file) => ({
                    path: file.path,
                    content: file.content
                })),
                ...removedFiles.map((path) => ({
                    path,
                    remove: true
                }))
            ];
        }

        return payload;
    }

    updatePayloadPreview() {
        const preview = document.getElementById('source-payload-preview');
        if (!preview) {
            return;
        }
        const mode = document.getElementById('form-deploy-mode').value;
        if (mode !== 'source') {
            preview.textContent = '';
            return;
        }
        const payload = this.buildSourcePayload();
        preview.textContent = JSON.stringify(payload, null, 2);
    }

    setSourceType(type) {
        const zipGroup = document.getElementById('source-zip-group');
        const gitGroup = document.getElementById('source-git-group');
        const gitMeta = document.getElementById('source-git-meta');
        const zipInput = document.getElementById('form-source-zip');
        const gitInput = document.getElementById('form-source-git-url');
        const gitRef = document.getElementById('form-source-git-ref');
        const sourcePath = document.getElementById('form-source-path');
        const runtimeGroup = document.getElementById('source-runtime-group');

        if (type === 'git') {
            zipGroup.style.display = 'none';
            gitGroup.style.display = 'block';
            gitMeta.style.display = 'flex';
            zipInput.required = false;
            gitInput.required = true;
            if (runtimeGroup) {
                runtimeGroup.style.display = 'block';
            }
            if (gitRef && !gitRef.value) {
                gitRef.value = 'main';
            }
            if (sourcePath && !sourcePath.value) {
                sourcePath.value = '.';
            }
        } else {
            zipGroup.style.display = 'block';
            gitGroup.style.display = 'none';
            gitMeta.style.display = 'none';
            zipInput.required = true;
            gitInput.required = false;
            if (runtimeGroup) {
                runtimeGroup.style.display = 'none';
            }
        }

        this.updatePayloadPreview();
        this.updateSourceBasicVisibility();
    }

    async submitFunctionForm(e) {
        e.preventDefault();

        const deployMode = document.getElementById('form-deploy-mode').value;
        const service = document.getElementById('form-service').value.trim();
        const image = document.getElementById('form-image').value.trim();
        const network = document.getElementById('form-network').value.trim();
        const envInput = document.getElementById('form-env').value.trim();
        const labelsInput = document.getElementById('form-labels').value.trim();
        const secretsInput = document.getElementById('form-secrets').value.trim();
        const memory = document.getElementById('form-memory').value.trim();
        const cpu = document.getElementById('form-cpu').value.trim();
        const readOnly = document.getElementById('form-readonly').checked;
        const debug = document.getElementById('form-debug').checked;
        const networkAuto = document.getElementById('form-network-auto').checked;

        if (deployMode === 'source') {
            if (!this.sourceLoaded) {
                this.showToast('Load source first to review docker-faas.yaml.', 'warning');
                return;
            }

            const sourceType = document.getElementById('form-source-type').value;
            const zipFile = document.getElementById('form-source-zip').files[0];
            const gitUrl = document.getElementById('form-source-git-url').value.trim();
            const gitRef = document.getElementById('form-source-git-ref').value.trim();
            const sourcePath = document.getElementById('form-source-path').value.trim();
            const runtime = document.getElementById('form-source-runtime').value;
            const manifestOverride = document.getElementById('form-source-manifest').value.trim();

            if (!service) {
                this.showToast('Function name is required', 'error');
                return;
            }

            if (sourceType === 'zip' && !zipFile) {
                this.showToast('Please select a zip file', 'error');
                return;
            }

            if (sourceType === 'git' && !gitUrl) {
                this.showToast('Please enter a Git URL', 'error');
                return;
            }

            const payload = this.buildSourcePayload();
            console.info('Source build request:', payload);
            this.updatePayloadPreview();

            try {
                const startedAt = performance.now();
                let response;
                if (sourceType === 'zip') {
                    const formData = new FormData();
                    formData.append('name', payload.name);
                    formData.append('runtime', runtime);
                    formData.append('sourceType', sourceType);
                    formData.append('deploy', 'true');
                    if (manifestOverride) {
                        formData.append('manifest', manifestOverride);
                    }
                    if (payload.source.files && payload.source.files.length > 0) {
                        formData.append('files', JSON.stringify(payload.source.files));
                    }
                    formData.append('file', zipFile);

                    response = await this.api('/system/builds', {
                        method: 'POST',
                        body: formData,
                        raw: true
                    });
                } else {
                    payload.deploy = true;
                    response = await this.api('/system/builds', {
                        method: 'POST',
                        body: JSON.stringify(payload)
                    });
                }

                const responseText = await response.text();
                const durationMs = Math.round(performance.now() - startedAt);
                if (response.ok) {
                    let result = null;
                    try {
                        result = JSON.parse(responseText);
                    } catch (error) {
                        result = null;
                    }
                    const verb = result && result.updated ? 'updated' : 'deployed';
                    const imageName = result?.image || 'image';
                    this.showToast(`Build ${verb}: ${imageName}`, 'success');
                    this.refreshBuildHistory();
                    this.switchView('functions');
                } else {
                    this.showToast(`Build failed: ${responseText}`, 'error');
                }
            } catch (error) {
                this.showToast(`Build error: ${error.message}`, 'error');
            }
            return;
        }

        const payload = {
            service,
            image
        };

        if (network && !networkAuto) payload.network = network;

        if (envInput) {
            try {
                payload.envVars = JSON.parse(envInput);
            } catch (e) {
                this.showToast('Invalid environment variables JSON', 'error');
                return;
            }
        }

        if (labelsInput) {
            try {
                payload.labels = JSON.parse(labelsInput);
            } catch (e) {
                this.showToast('Invalid labels JSON', 'error');
                return;
            }
        }

        if (secretsInput) {
            payload.secrets = secretsInput.split(',').map(s => s.trim()).filter(s => s);
        }

        if (memory || cpu) {
            payload.limits = {};
            if (memory) payload.limits.memory = memory;
            if (cpu) payload.limits.cpu = cpu;
        }

        payload.readOnlyRootFilesystem = readOnly;
        payload.debug = debug;

        const isUpdate = this.currentFunction !== null;
        const endpoint = '/system/functions';
        const method = isUpdate ? 'PUT' : 'POST';

        try {
            const response = await this.api(endpoint, {
                method,
                body: JSON.stringify(payload)
            });

            if (response.ok) {
                this.showToast(`Function ${isUpdate ? 'updated' : 'created'} successfully`, 'success');
                this.switchView('functions');
            } else {
                const error = await response.text();
                this.showToast(`Failed to ${isUpdate ? 'update' : 'create'} function: ${error}`, 'error');
            }
        } catch (error) {
            this.showToast(`Error: ${error.message}`, 'error');
        }
    }

    // Secrets
    async loadSecrets() {
        try {
            const response = await this.api('/system/secrets');
            if (response.ok) {
                this.secrets = await response.json();
                this.renderSecretsTable();
            } else {
                this.showToast('Failed to load secrets', 'error');
            }
        } catch (error) {
            this.showToast(`Error loading secrets: ${error.message}`, 'error');
        }
    }

    renderSecretsTable() {
        const tbody = document.getElementById('secrets-tbody');

        if (this.secrets.length === 0) {
            tbody.innerHTML = '<tr><td colspan="2" class="empty-state">No secrets</td></tr>';
            return;
        }

        tbody.innerHTML = this.secrets.map(secret => `
            <tr>
                <td><strong>${secret.name}</strong></td>
                <td>
                    <button class="btn btn-secondary btn-sm" onclick="app.updateSecret('${secret.name}')">Update</button>
                    <button class="btn btn-danger btn-sm" onclick="app.deleteSecret('${secret.name}')">Delete</button>
                </td>
            </tr>
        `).join('');
    }

    showSecretModal(secretName = null) {
        const modal = document.getElementById('secret-modal');
        const title = document.getElementById('secret-modal-title');
        const nameInput = document.getElementById('secret-name');
        const valueInput = document.getElementById('secret-value');
        const errorDiv = document.getElementById('secret-error');

        if (secretName) {
            title.textContent = 'Update Secret';
            nameInput.value = secretName;
            nameInput.disabled = true;
            valueInput.value = '';
        } else {
            title.textContent = 'Create Secret';
            nameInput.value = '';
            nameInput.disabled = false;
            valueInput.value = '';
        }

        errorDiv.textContent = '';
        errorDiv.classList.remove('show');
        modal.classList.add('active');
    }

    hideSecretModal() {
        document.getElementById('secret-modal').classList.remove('active');
    }

    updateSecret(name) {
        this.showSecretModal(name);
    }

    async saveSecret() {
        const name = document.getElementById('secret-name').value.trim();
        const value = document.getElementById('secret-value').value;

        if (!name || !value) {
            this.showError('secret-error', 'Name and value are required');
            return;
        }

        const isUpdate = document.getElementById('secret-name').disabled;
        const endpoint = '/system/secrets';
        const method = isUpdate ? 'PUT' : 'POST';

        try {
            const response = await this.api(endpoint, {
                method,
                body: JSON.stringify({ name, value })
            });

            if (response.ok) {
                this.showToast(`Secret ${isUpdate ? 'updated' : 'created'} successfully`, 'success');
                this.hideSecretModal();
                this.loadSecrets();
            } else {
                const error = await response.text();
                this.showError('secret-error', `Failed: ${error}`);
            }
        } catch (error) {
            this.showError('secret-error', error.message);
        }
    }

    async deleteSecret(name) {
        if (!confirm(`Are you sure you want to delete secret "${name}"?`)) {
            return;
        }

        try {
            const response = await this.api(`/system/secrets?name=${encodeURIComponent(name)}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                this.showToast('Secret deleted successfully', 'success');
                this.loadSecrets();
            } else {
                const error = await response.text();
                this.showToast(`Failed to delete secret: ${error}`, 'error');
            }
        } catch (error) {
            this.showToast(`Error deleting secret: ${error.message}`, 'error');
        }
    }

    // Logs
    async loadLogsView() {
        const select = document.getElementById('logs-function');
        select.innerHTML = '<option value="">Select function...</option>';

        try {
            await this.fetchFunctions();
        } catch (error) {
            this.showToast(`Error loading functions: ${error.message}`, 'error');
            return;
        }

        this.functions.forEach(fn => {
            const option = document.createElement('option');
            option.value = fn.name;
            option.textContent = fn.name;
            select.appendChild(option);
        });
    }

    async fetchLogs() {
        const functionName = document.getElementById('logs-function').value;
        const tail = document.getElementById('logs-tail').value;

        if (!functionName) {
            this.showToast('Please select a function', 'warning');
            return;
        }

        try {
            const response = await this.api(`/system/logs?name=${encodeURIComponent(functionName)}&tail=${tail}`, {
                showLoading: true
            });
            if (response.ok) {
                const logs = await response.text();
                document.getElementById('logs-output').textContent = logs || 'No logs available';
            } else {
                const error = await response.text();
                document.getElementById('logs-output').textContent = `Error: ${error}`;
            }
        } catch (error) {
            document.getElementById('logs-output').textContent = `Error fetching logs: ${error.message}`;
        }
    }

    // Build Activity
    loadBuildHistoryView() {
        this.refreshBuildHistory();
        if (this.currentBuildId) {
            const entry = this.buildHistory.find((item) => item.id === this.currentBuildId);
            if (entry) {
                this.renderBuildDetails(entry);
            } else {
                this.hideBuildDetails();
            }
            return;
        }
        this.hideBuildDetails();
    }

    renderBuildsTable() {
        const tbody = document.getElementById('builds-tbody');
        if (!tbody) {
            return;
        }

        if (this.buildHistory.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="empty-state">No builds recorded</td></tr>';
            return;
        }

        tbody.innerHTML = this.buildHistory.map((entry) => {
            const statusBadge = this.formatBuildStatus(entry.status);
            const startedAt = this.formatDate(entry.startedAt);
            const duration = entry.durationMs ? `${(entry.durationMs / 1000).toFixed(1)}s` : '-';
            const sourceLabel = entry.sourceType ? entry.sourceType : '-';
            const image = entry.image || '-';
            return `
                <tr>
                    <td>${startedAt || '-'}</td>
                    <td><strong>${entry.name || '-'}</strong></td>
                    <td><code>${sourceLabel}</code></td>
                    <td>${statusBadge}</td>
                    <td><code>${image}</code></td>
                    <td>${duration}</td>
                    <td>
                        <button class="btn btn-secondary btn-sm" onclick="app.showBuildDetails('${entry.id}')">View</button>
                    </td>
                </tr>
            `;
        }).join('');
    }

    showBuildDetails(id) {
        const entry = this.buildHistory.find((item) => item.id === id);
        if (!entry) {
            return;
        }
        this.currentBuildId = id;
        this.renderBuildDetails(entry);
        const panel = document.getElementById('build-detail-panel');
        if (panel) {
            panel.style.display = 'block';
        }
    }

    hideBuildDetails() {
        this.currentBuildId = null;
        const panel = document.getElementById('build-detail-panel');
        if (panel) {
            panel.style.display = 'none';
        }
    }

    renderBuildDetails(entry) {
        const list = document.getElementById('build-detail-list');
        const output = document.getElementById('build-detail-output');
        if (!list || !output) {
            return;
        }

        const items = [
            ['Status', entry.status || '-'],
            ['Function', entry.name || '-'],
            ['Source', entry.sourceType || '-'],
            ['Runtime', entry.runtime || '-'],
            ['Git URL', entry.gitUrl || '-'],
            ['Git Ref', entry.gitRef || '-'],
            ['Subdirectory', entry.sourcePath || '-'],
            ['Zip File', entry.zipName || '-'],
            ['Image', entry.image || '-'],
            ['Deployed', this.formatBool(entry.deployed)],
            ['Updated', this.formatBool(entry.updated)],
            ['Started', entry.startedAt ? this.formatDate(entry.startedAt) : '-'],
            ['Finished', entry.finishedAt ? this.formatDate(entry.finishedAt) : '-'],
            ['Duration', entry.durationMs ? `${(entry.durationMs / 1000).toFixed(1)}s` : '-']
        ];

        list.innerHTML = items.map(([label, value]) => `
            <dt>${label}</dt>
            <dd>${value}</dd>
        `).join('');

        const outputLines = [];
        if (entry.output) {
            outputLines.push(entry.output);
        }
        if (entry.error) {
            outputLines.push(`Error: ${entry.error}`);
        }
        if (entry.manifest) {
            outputLines.push('--- docker-faas.yaml ---');
            outputLines.push(entry.manifest);
        }
        if (entry.fileChanges && entry.fileChanges.length > 0) {
            outputLines.push('--- File Changes ---');
            outputLines.push(entry.fileChanges.join('\n'));
        }
        if (entry.truncated) {
            outputLines.push('--- Output truncated ---');
        }
        output.textContent = outputLines.length > 0 ? outputLines.join('\n') : 'No build output available.';
    }

    formatBuildStatus(status) {
        switch (status) {
            case 'success':
                return '<span class="badge badge-success">Success</span>';
            case 'failed':
                return '<span class="badge badge-danger">Failed</span>';
            case 'running':
                return '<span class="badge badge-warning">Running</span>';
            default:
                return '<span class="badge badge-info">Pending</span>';
        }
    }

    // Backup and Import
    async exportFunctions() {
        try {
            const functions = await this.fetchFunctions();
            const exportData = {
                version: '1',
                exportedAt: new Date().toISOString(),
                gateway: this.gatewayUrl,
                functions: functions.map((fn) => ({
                    name: fn.name,
                    image: fn.image,
                    network: fn.network || '',
                    envProcess: fn.envProcess || '',
                    envVars: fn.envVars || {},
                    labels: fn.labels || {},
                    secrets: fn.secrets || [],
                    limits: fn.limits || null,
                    requests: fn.requests || null,
                    readOnlyRootFilesystem: !!fn.readOnlyRootFilesystem,
                    debug: !!fn.debug
                }))
            };

            const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = url;
            link.download = `docker-faas-backup-${Date.now()}.json`;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(url);
            this.showToast('Export ready', 'success');
        } catch (error) {
            this.showToast(`Export failed: ${error.message}`, 'error');
        }
    }

    triggerImport() {
        const input = document.getElementById('import-functions-file');
        if (!input) {
            return;
        }
        input.value = '';
        input.click();
    }

    async handleImportFile(file) {
        if (!file) {
            return;
        }
        try {
            const text = await file.text();
            const data = JSON.parse(text);
            const plan = this.buildImportPlan(data);
            if (plan.functions.length === 0) {
                this.showToast('No functions found in import file', 'warning');
                return;
            }
            this.importPlan = plan;
            await this.renderImportSummary(plan);
            this.showImportModal();
        } catch (error) {
            this.showToast(`Import failed: ${error.message}`, 'error');
        }
    }

    buildImportPlan(data) {
        if (!data || !Array.isArray(data.functions)) {
            throw new Error('Invalid import file format');
        }
        const functions = data.functions.map((fn) => {
            const name = (fn.service || fn.name || '').trim();
            return {
                name,
                image: fn.image || '',
                network: fn.network || '',
                envProcess: fn.envProcess || '',
                envVars: fn.envVars || {},
                labels: fn.labels || {},
                secrets: Array.isArray(fn.secrets) ? fn.secrets : [],
                limits: fn.limits || null,
                requests: fn.requests || null,
                readOnlyRootFilesystem: !!fn.readOnlyRootFilesystem,
                debug: !!fn.debug
            };
        }).filter((fn) => fn.name);

        return {
            version: data.version || '1',
            functions
        };
    }

    async renderImportSummary(plan) {
        const summary = document.getElementById('import-summary');
        const errorDiv = document.getElementById('import-error');
        if (!summary || !errorDiv) {
            return;
        }
        errorDiv.textContent = '';
        errorDiv.classList.remove('show');

        const secrets = await this.fetchSecretNames();
        const missingSecrets = new Set();

        const listItems = plan.functions.map((fn) => {
            const missing = fn.secrets.filter((secret) => !secrets.includes(secret));
            missing.forEach((secret) => missingSecrets.add(secret));
            const detailParts = [];
            if (fn.image) {
                detailParts.push(fn.image);
            }
            if (fn.secrets.length > 0) {
                detailParts.push(`secrets: ${fn.secrets.join(', ')}`);
            }
            if (missing.length > 0) {
                detailParts.push(`missing secrets: ${missing.join(', ')}`);
            }
            const details = detailParts.length > 0 ? ` (${detailParts.join(' | ')})` : '';
            return `<li><strong>${fn.name}</strong>${details}</li>`;
        });

        const warning = missingSecrets.size > 0
            ? `Missing secrets: ${Array.from(missingSecrets).join(', ')}`
            : '';

        summary.innerHTML = `
            <div><strong>${plan.functions.length}</strong> functions ready to import.</div>
            ${warning ? `<div class="text-muted">${warning}</div>` : ''}
            <ul>${listItems.join('')}</ul>
        `;
    }

    showImportModal() {
        const modal = document.getElementById('import-modal');
        if (modal) {
            modal.classList.add('active');
        }
    }

    hideImportModal() {
        const modal = document.getElementById('import-modal');
        if (modal) {
            modal.classList.remove('active');
        }
        this.importPlan = null;
    }

    async confirmImport() {
        if (!this.importPlan) {
            return;
        }

        const results = { success: [], failed: [] };
        const existing = new Set((await this.fetchFunctions()).map((fn) => fn.name));

        this.showLoading();
        try {
            for (const fn of this.importPlan.functions) {
                const payload = {
                    service: fn.name,
                    image: fn.image,
                    envProcess: fn.envProcess || undefined,
                    envVars: fn.envVars,
                    labels: fn.labels,
                    secrets: fn.secrets,
                    limits: fn.limits || undefined,
                    requests: fn.requests || undefined,
                    readOnlyRootFilesystem: fn.readOnlyRootFilesystem,
                    debug: fn.debug
                };
                if (fn.network) {
                    payload.network = fn.network;
                }

                const method = existing.has(fn.name) ? 'PUT' : 'POST';
                const response = await this.api('/system/functions', {
                    method,
                    body: JSON.stringify(payload),
                    showLoading: false
                });

                if (response.ok) {
                    results.success.push(fn.name);
                } else {
                    const errorText = await response.text();
                    results.failed.push({ name: fn.name, error: errorText });
                }
            }
        } catch (error) {
            this.showToast(`Import error: ${error.message}`, 'error');
        } finally {
            this.hideLoading();
        }

        const summary = document.getElementById('import-summary');
        if (summary) {
            const failures = results.failed.map((item) => `<li>${item.name}: ${item.error}</li>`).join('');
            summary.innerHTML = `
                <div><strong>${results.success.length}</strong> functions imported.</div>
                ${results.failed.length > 0 ? `<div class="text-muted">Failures: ${results.failed.length}</div><ul>${failures}</ul>` : ''}
            `;
        }

        if (results.success.length > 0) {
            this.loadFunctions();
        }
        if (results.failed.length === 0) {
            this.showToast('Import completed', 'success');
        } else {
            this.showToast('Import completed with errors', 'warning');
        }
    }

    async fetchSecretNames() {
        try {
            const response = await this.api('/system/secrets', { showLoading: false });
            if (!response.ok) {
                return [];
            }
            const secrets = await response.json();
            return secrets.map((secret) => secret.name);
        } catch (error) {
            return [];
        }
    }

    // Metrics
    async loadMetrics() {
        const rawTarget = document.getElementById('metrics-raw');
        try {
            const response = await this.api('/system/metrics', {
                headers: { Accept: 'text/plain' },
                showLoading: true,
                raw: true
            });
            if (!response.ok) {
                const errorText = await response.text();
                if (rawTarget) {
                    rawTarget.textContent = `Failed to load metrics: ${errorText}`;
                }
                this.showToast('Failed to load metrics', 'error');
                return;
            }
            const raw = await response.text();
            if (rawTarget) {
                rawTarget.textContent = raw.trim() || 'No metrics returned.';
            }

            const parsed = this.parsePrometheusMetrics(raw);
            this.renderMetrics(parsed);
            const health = await this.fetchHealthChecks();
            if (health) {
                this.renderHealthChecks(health);
            }
        } catch (error) {
            if (rawTarget) {
                rawTarget.textContent = `Failed to load metrics: ${error.message}`;
            }
            this.showToast(`Metrics error: ${error.message}`, 'error');
        }
    }

    parsePrometheusMetrics(text) {
        const samples = [];
        const lines = text.split('\n');
        const lineRegex = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([-0-9.eE+]+)$/;
        const labelRegex = /([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*)"/g;

        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed || trimmed.startsWith('#')) {
                continue;
            }
            const match = trimmed.match(lineRegex);
            if (!match) {
                continue;
            }
            const name = match[1];
            const labelPart = match[2];
            const value = parseFloat(match[3]);
            if (Number.isNaN(value)) {
                continue;
            }
            const labels = {};
            if (labelPart) {
                let labelMatch;
                while ((labelMatch = labelRegex.exec(labelPart)) !== null) {
                    labels[labelMatch[1]] = labelMatch[2];
                }
            }
            samples.push({ name, labels, value });
        }

        const byName = {};
        samples.forEach((sample) => {
            if (!byName[sample.name]) {
                byName[sample.name] = [];
            }
            byName[sample.name].push(sample);
        });

        return { samples, byName };
    }

    renderMetrics(parsed) {
        const totalRequests = this.sumMetric(parsed, 'gateway_http_requests_total');
        const totalErrors = this.sumMetric(parsed, 'gateway_http_errors_total');
        const totalInvocations = this.sumMetric(parsed, 'function_invocations_total');
        const functionsDeployed = this.firstMetric(parsed, 'functions_deployed');

        document.getElementById('metric-gateway-requests').textContent = this.formatNumber(totalRequests);
        document.getElementById('metric-gateway-errors').textContent = this.formatNumber(totalErrors);
        document.getElementById('metric-function-invocations').textContent = this.formatNumber(totalInvocations);
        document.getElementById('metric-functions-deployed').textContent = this.formatNumber(functionsDeployed);

        const functionTotals = {};
        const errorTotals = {};
        (parsed.byName['function_invocations_total'] || []).forEach((sample) => {
            const name = sample.labels.function_name || 'unknown';
            functionTotals[name] = (functionTotals[name] || 0) + sample.value;
        });
        (parsed.byName['function_errors_total'] || []).forEach((sample) => {
            const name = sample.labels.function_name || 'unknown';
            errorTotals[name] = (errorTotals[name] || 0) + sample.value;
        });

        const top = Object.keys(functionTotals)
            .map((name) => ({
                name,
                invocations: functionTotals[name],
                errors: errorTotals[name] || 0
            }))
            .sort((a, b) => b.invocations - a.invocations)
            .slice(0, 5);

        const list = document.getElementById('metrics-function-list');
        if (list) {
            if (top.length === 0) {
                list.innerHTML = '<div class="text-muted">No function metrics yet.</div>';
            } else {
                list.innerHTML = top.map((item) => `
                    <div class="metrics-row">
                        <strong>${item.name}</strong>
                        <span>${this.formatNumber(item.invocations)} invocations / ${this.formatNumber(item.errors)} errors</span>
                    </div>
                `).join('');
            }
        }
    }

    sumMetric(parsed, name) {
        const samples = parsed.byName[name] || [];
        return samples.reduce((sum, sample) => sum + sample.value, 0);
    }

    firstMetric(parsed, name) {
        const samples = parsed.byName[name] || [];
        if (samples.length === 0) {
            return 0;
        }
        return samples[0].value;
    }

    async fetchHealthChecks() {
        try {
            const response = await this.api('/healthz', {
                headers: { Accept: 'application/json' },
                showLoading: false
            });
            if (!response.ok) {
                return null;
            }
            return await response.json();
        } catch (error) {
            return null;
        }
    }

    renderHealthChecks(health) {
        const list = document.getElementById('health-check-list');
        const updated = document.getElementById('health-check-updated');
        if (!list || !health || !health.checks) {
            return;
        }
        const items = Object.entries(health.checks).map(([name, status]) => {
            const ok = status === 'ok';
            return `
                <div class="status-item ${ok ? 'ok' : 'error'}">
                    <span>${name}</span>
                    <span>${ok ? 'ok' : status}</span>
                </div>
            `;
        });
        list.innerHTML = items.join('');
        if (updated) {
            updated.textContent = `Last updated: ${new Date().toLocaleString()}`;
        }
    }

    // Settings
    async loadSettings() {
        try {
            const response = await this.api('/system/config', { showLoading: false });
            if (!response.ok) {
                const errorText = await response.text();
                this.showToast(`Failed to load settings: ${errorText}`, 'error');
                return;
            }
            const config = await response.json();
            if (Number.isFinite(config.buildHistoryLimit) && config.buildHistoryLimit > 0) {
                this.buildHistoryLimit = config.buildHistoryLimit;
            }
            this.renderSettings(config);
        } catch (error) {
            this.showToast(`Settings error: ${error.message}`, 'error');
        }
    }

    renderSettings(config) {
        const list = document.getElementById('settings-list');
        if (!list) {
            return;
        }

        const entries = [
            ['Auth Enabled', this.formatBool(config.authEnabled)],
            ['Require Auth for Functions', this.formatBool(config.requireAuthForFunctions)],
            ['CORS Allowed Origins', Array.isArray(config.corsAllowedOrigins) ? config.corsAllowedOrigins.join(', ') : '-'],
            ['Functions Network', config.functionsNetwork || '-'],
            ['Default Replicas', this.formatNumber(config.defaultReplicas)],
            ['Max Replicas', this.formatNumber(config.maxReplicas)],
            ['Metrics Enabled', this.formatBool(config.metricsEnabled)],
            ['Metrics Port', config.metricsPort || '-'],
            ['Debug Bind Address', config.debugBindAddress || '-'],
            ['Auth Rate Limit', this.formatNumber(config.authRateLimit)],
            ['Auth Rate Window (s)', this.formatNumber(config.authRateWindowSeconds)],
            ['Auth Token TTL (s)', this.formatNumber(config.authTokenTTLSeconds)],
            ['Build History Limit', this.formatNumber(config.buildHistoryLimit)],
            ['Build History Retention (s)', this.formatNumber(config.buildHistoryRetentionSeconds)],
            ['Build Output Limit (bytes)', this.formatNumber(config.buildOutputLimit)]
        ];

        list.innerHTML = entries.map(([label, value]) => `
            <dt>${label}</dt>
            <dd>${value}</dd>
        `).join('');
    }

    // UI Helpers
    showLoading() {
        this.loadingCount += 1;
        const overlay = document.getElementById('loading-overlay');
        if (overlay) {
            overlay.classList.add('active');
        }
    }

    resetLoadingState() {
        this.loadingCount = 0;
        const overlay = document.getElementById('loading-overlay');
        if (overlay) {
            overlay.classList.remove('active');
        }
    }

    hideLoading() {
        this.loadingCount = Math.max(0, this.loadingCount - 1);
        if (this.loadingCount > 0) {
            return;
        }
        const overlay = document.getElementById('loading-overlay');
        if (overlay) {
            overlay.classList.remove('active');
        }
    }

    showError(elementId, message) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = message;
            element.classList.add('show');
        }
    }

    formatDate(value) {
        if (!value) {
            return '';
        }
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) {
            return '';
        }
        return date.toLocaleString();
    }

    isTokenExpired(expiresAt) {
        if (!expiresAt) {
            return false;
        }
        const timeValue = new Date(expiresAt).getTime();
        if (Number.isNaN(timeValue)) {
            return false;
        }
        return timeValue <= Date.now();
    }

    formatNumber(value) {
        if (value === null || value === undefined) {
            return '-';
        }
        if (!Number.isFinite(value)) {
            return '-';
        }
        return new Intl.NumberFormat().format(value);
    }

    formatKeyList(map) {
        if (!map || Object.keys(map).length === 0) {
            return '';
        }
        return Object.keys(map).join(', ');
    }

    formatResources(resources) {
        if (!resources) {
            return '';
        }
        const parts = [];
        if (resources.memory) {
            parts.push(`Memory: ${resources.memory}`);
        }
        if (resources.cpu) {
            parts.push(`CPU: ${resources.cpu}`);
        }
        return parts.join(' | ');
    }

    formatBool(value) {
        if (value === undefined || value === null) {
            return '-';
        }
        return value ? 'Yes' : 'No';
    }

    isReplicaHealthy(status) {
        const normalized = (status || '').toLowerCase();
        return normalized.includes('running') || normalized.includes('up');
    }

    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(() => {
            toast.remove();
        }, 5000);
    }
}

// Initialize app
const app = new DockerFaaSApp();
