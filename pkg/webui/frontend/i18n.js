// Internationalization support
const i18n = {
    en: {
        title: 'External Tools Manager',
        loading: 'Loading tools...',
        version: 'Version',
        installed: 'Installed',
        preinstalled: 'Preinstalled',
        notInstalled: 'Not Installed',
        installing: 'Installing...',
        uninstalling: 'Uninstalling...',
        install: 'Install',
        uninstall: 'Uninstall',
        notUninstallable: 'Cannot uninstall',
        pause: 'Pause',
        resume: 'Resume',
        reinstall: 'Reinstall',
        downloading: 'Downloading',
        extracting: 'Extracting files...',
        completed: 'Installation completed',
        uninstallCompleted: 'Uninstallation completed',
        failed: 'Installation Failed',
        uninstallFailed: 'Uninstallation Failed',
        error: 'Error',
        failedToLoad: 'Failed to load tools',
        folder: 'Folder',
        showInfo: 'Show Info',
        hideInfo: 'Hide Info'
    },
    zh: {
        title: '外部工具管理器',
        loading: '加载工具中...',
        version: '版本',
        installed: '已安装',
        preinstalled: '预装',
        notInstalled: '未安装',
        installing: '安装中...',
        uninstalling: '卸载中...',
        install: '安装',
        uninstall: '卸载',
        notUninstallable: '不可卸载',
        pause: '暂停',
        resume: '继续',
        reinstall: '重新安装',
        downloading: '下载中',
        extracting: '解压中...',
        completed: '安装完成',
        uninstallCompleted: '卸载完成',
        failed: '安装失败',
        uninstallFailed: '卸载失败',
        error: '错误',
        failedToLoad: '加载工具失败',
        folder: '目录',
        showInfo: '查看信息',
        hideInfo: '隐藏信息',
        nonExecutable: '非可执行程序',
        execFromTemp: '临时目录运行'
    }
};

// Get user's preferred language
function getPreferredLanguage() {
    // Check localStorage first
    const saved = localStorage.getItem('language');
    if (saved && (saved === 'en' || saved === 'zh')) {
        return saved;
    }

    // Check browser language
    const browserLang = navigator.language || navigator.userLanguage;
    if (browserLang.startsWith('zh')) {
        return 'zh';
    }

    return 'en';
}

// Set language
function setLanguage(lang) {
    currentLanguage = lang;
    localStorage.setItem('language', lang);

    // Update all elements with data-i18n attribute
    document.querySelectorAll('[data-i18n]').forEach(element => {
        const key = element.getAttribute('data-i18n');
        if (i18n[lang][key]) {
            element.textContent = i18n[lang][key];
        }
    });

    // Update language buttons
    document.querySelectorAll('.lang-btn').forEach(btn => {
        btn.classList.toggle('active', btn.getAttribute('data-lang') === lang);
    });

    // Update HTML lang attribute
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
}

// Get translation
function t(key) {
    return i18n[currentLanguage][key] || key;
}

// Initialize language
let currentLanguage = getPreferredLanguage();

// Set up language switcher when DOM is loaded
document.addEventListener('DOMContentLoaded', function () {
    setLanguage(currentLanguage);

    document.querySelectorAll('.lang-btn').forEach(btn => {
        btn.addEventListener('click', function () {
            setLanguage(this.getAttribute('data-lang'));
            // Re-render tools with new language
            if (window.toolsData && window.toolsData.length > 0) {
                renderTools(window.toolsData);
            }
        });
    });
});
