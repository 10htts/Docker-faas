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
        this.defaultGatewayUrl = this.getDefaultGatewayUrl();

        this.init();
    }

    init() {
        this.bindEvents();
        this.prefillGatewayUrl();
        this.checkSession();
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
            item.addEventListener('click', (e) => this.switchView(e.target.dataset.view));
        });

        // Overview quick actions
        document.getElementById('deploy-new-btn')?.addEventListener('click', () => this.showCreateFunction());
        document.getElementById('deploy-source-btn')?.addEventListener('click', () => this.showCreateFunction('source'));
        document.getElementById('view-functions-btn')?.addEventListener('click', () => this.switchView('functions'));
        document.getElementById('manage-secrets-btn')?.addEventListener('click', () => this.switchView('secrets'));

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
            const data = JSON.parse(session);
            this.gatewayUrl = this.normalizeGatewayUrl(data.gatewayUrl || this.defaultGatewayUrl);
            this.username = data.username;
            this.password = data.password;
            this.authenticated = true;
            this.showApp();
            this.loadOverview();
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

        // Test connection
        try {
            const response = await this.api('/system/info');
            if (response.ok) {
                this.authenticated = true;
                localStorage.setItem('dockerfaas-session', JSON.stringify({
                    gatewayUrl: this.gatewayUrl,
                    username: this.username,
                    password: this.password
                }));
                this.showApp();
                this.loadOverview();
                this.showToast('Connected successfully', 'success');
            } else {
                this.showError('login-error', 'Authentication failed');
            }
        } catch (error) {
            this.showError('login-error', `Connection failed: ${error.message}`);
        }
    }

    logout() {
        this.authenticated = false;
        localStorage.removeItem('dockerfaas-session');
        this.hideApp();
        this.showToast('Disconnected', 'success');
    }

    showApp() {
        document.getElementById('login-screen').classList.remove('active');
        document.getElementById('app-screen').classList.add('active');
        document.getElementById('header-gateway-url').textContent = this.gatewayUrl;
        this.startAutoRefresh();
    }

    hideApp() {
        document.getElementById('app-screen').classList.remove('active');
        document.getElementById('login-screen').classList.add('active');
        this.stopAutoRefresh();
    }

    // API Methods
    async api(endpoint, options = {}) {
        const url = `${this.gatewayUrl}${endpoint}`;
        const headers = {
            'Authorization': 'Basic ' + btoa(`${this.username}:${this.password}`),
            ...options.headers
        };

        const hasContentType = Object.keys(headers).some((key) => key.toLowerCase() === 'content-type');
        if (!options.raw && options.body !== undefined && !hasContentType) {
            headers['Content-Type'] = 'application/json';
        }

        return fetch(url, {
            ...options,
            headers
        });
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
            case 'secrets':
                this.loadSecrets();
                break;
            case 'logs':
                this.loadLogsView();
                break;
        }
    }

    refreshCurrentView() {
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

    // Overview
    async loadOverview() {
        try {
            // Load system info
            const infoResponse = await this.api('/system/info');
            if (infoResponse.ok) {
                const info = await infoResponse.json();
                const version = info.version?.release || info.version?.sha || info.provider?.version || 'v2.0';
                document.getElementById('stat-version').textContent = version;
                this.setHealthStatus('healthy', 'Healthy');
            } else {
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
        document.getElementById('form-network-auto').checked = true;
        this.toggleNetworkAuto(true);
        document.getElementById('form-deploy-mode').value = mode;
        this.setDeployMode(mode);

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
        if (!trimmed || this.serviceTouched) {
            return;
        }
        const repoPart = trimmed.split('/').filter(Boolean).pop() || '';
        const name = repoPart.replace(/\.git$/, '');
        if (name) {
            this.setServiceValue(name);
        }
    }

    handleZipInput(file) {
        if (!file || this.serviceTouched) {
            return;
        }
        const name = file.name.replace(/\.zip$/i, '');
        if (name) {
            this.setServiceValue(name);
        }
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
            this.setSectionDisabled(envSection, true);
            this.setSectionDisabled(labelsSection, true);
            this.setSectionDisabled(secretsSection, true);
            this.setSectionDisabled(limitsSection, true);
            this.setSectionDisabled(advancedSection, true);
            if (networkAutoToggle) {
                networkAutoToggle.checked = true;
                this.toggleNetworkAuto(true);
            }
        } else {
            imageSection.style.display = 'block';
            sourceSection.style.display = 'none';
            imageInput.required = true;
            submitBtn.textContent = 'Deploy Function';
            sourceWarning.style.display = 'none';
            this.setSectionDisabled(envSection, false);
            this.setSectionDisabled(labelsSection, false);
            this.setSectionDisabled(secretsSection, false);
            this.setSectionDisabled(limitsSection, false);
            this.setSectionDisabled(advancedSection, false);
        }
    }

    setSectionDisabled(section, disabled) {
        if (!section) {
            return;
        }
        section.classList.toggle('disabled', disabled);
        section.querySelectorAll('input, textarea, select, button').forEach((el) => {
            el.disabled = disabled;
        });
    }

    setSourceType(type) {
        const zipGroup = document.getElementById('source-zip-group');
        const gitGroup = document.getElementById('source-git-group');
        const gitMeta = document.getElementById('source-git-meta');
        const zipInput = document.getElementById('form-source-zip');
        const gitInput = document.getElementById('form-source-git-url');
        const gitRef = document.getElementById('form-source-git-ref');
        const sourcePath = document.getElementById('form-source-path');

        if (type === 'git') {
            zipGroup.style.display = 'none';
            gitGroup.style.display = 'block';
            gitMeta.style.display = 'flex';
            zipInput.required = false;
            gitInput.required = true;
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
        }
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

            console.info('Source build request:', {
                service,
                sourceType,
                runtime,
                gitUrl,
                gitRef,
                sourcePath,
                zipFileName: zipFile ? zipFile.name : null,
                manifestOverrideProvided: manifestOverride.length > 0
            });

            this.showToast('Source builds require the build API (/system/builds) and a builder service.', 'warning');
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
            const response = await this.api(`/system/logs?name=${encodeURIComponent(functionName)}&tail=${tail}`);
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

    // UI Helpers
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
