const docsList = [
    { file: 'GETTING_STARTED.md', title: 'Getting Started' },
    { file: 'API.md', title: 'API Reference' },
    { file: 'WEB_UI.md', title: 'Web UI Guide' },
    { file: 'V2_ENHANCEMENTS.md', title: 'v2.0 Enhancements' },
    { file: 'DEPLOYMENT.md', title: 'Production Deployment' },
    { file: 'SECRETS.md', title: 'Secrets Management' },
    { file: 'ARCHITECTURE.md', title: 'Architecture' },
    { file: 'SOURCE_PACKAGING.md', title: 'Source Packaging' },
    { file: 'SOURCE_EXAMPLES.md', title: 'Source Examples' },
];

const defaultDoc = 'GETTING_STARTED.md';

function getDocParam() {
    const params = new URLSearchParams(window.location.search);
    const file = params.get('file');
    return sanitizeDocFile(file);
}

function sanitizeDocFile(file) {
    if (!file) {
        return '';
    }
    const valid = /^[A-Za-z0-9_.-]+\.md$/;
    return valid.test(file) ? file : '';
}

function buildNav(activeFile) {
    const nav = document.getElementById('docs-nav');
    nav.innerHTML = '';
    docsList.forEach((doc) => {
        const link = document.createElement('a');
        link.href = `/ui/docs.html?file=${encodeURIComponent(doc.file)}`;
        link.textContent = doc.title;
        link.className = 'docs-nav-link';
        if (doc.file === activeFile) {
            link.classList.add('active');
        }
        nav.appendChild(link);
    });
}

function setMeta(title, file) {
    const meta = document.getElementById('docs-meta');
    meta.textContent = `${title} - ${file}`;
    document.title = `Docker FaaS Docs - ${title}`;
}

function rewriteLinks(container) {
    const anchors = container.querySelectorAll('a');
    anchors.forEach((anchor) => {
        const href = anchor.getAttribute('href') || '';
        if (!href) {
            return;
        }

        if (href.startsWith('http://') || href.startsWith('https://')) {
            anchor.target = '_blank';
            anchor.rel = 'noopener';
            return;
        }

        if (href.endsWith('.md')) {
            const file = sanitizeDocFile(href.split('/').pop());
            if (file) {
                anchor.href = `/ui/docs.html?file=${encodeURIComponent(file)}`;
            }
        }
    });
}

async function loadDoc(file) {
    const article = document.getElementById('docs-article');
    article.innerHTML = '<p class="docs-loading">Loading documentation...</p>';

    const response = await fetch(`/docs/${encodeURIComponent(file)}`);
    if (!response.ok) {
        article.innerHTML = `<p class="docs-error">Unable to load ${file}. (${response.status})</p>`;
        return;
    }

    const markdown = await response.text();
    const html = DOMPurify.sanitize(marked.parse(markdown));
    article.innerHTML = html;
    rewriteLinks(article);
}

function initDocs() {
    marked.setOptions({
        breaks: true,
        mangle: false,
        headerIds: true,
    });

    const file = getDocParam() || defaultDoc;
    const docMeta = docsList.find((doc) => doc.file === file) || { title: file };
    buildNav(file);
    setMeta(docMeta.title, file);
    loadDoc(file);

    const search = document.getElementById('docs-search');
    search.addEventListener('input', (event) => {
        const query = event.target.value.toLowerCase();
        const filtered = docsList.filter((doc) => doc.title.toLowerCase().includes(query));
        const nav = document.getElementById('docs-nav');
        nav.innerHTML = '';
        filtered.forEach((doc) => {
            const link = document.createElement('a');
            link.href = `/ui/docs.html?file=${encodeURIComponent(doc.file)}`;
            link.textContent = doc.title;
            link.className = 'docs-nav-link';
            if (doc.file === file) {
                link.classList.add('active');
            }
            nav.appendChild(link);
        });
    });
}

initDocs();
