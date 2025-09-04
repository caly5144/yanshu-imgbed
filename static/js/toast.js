// 创建一个美化的弹窗系统
class BeautifulAlert {
    constructor() {
        this.createStyles();
        this.createContainer();
    }

    createStyles() {
        if (document.getElementById('beautiful-alert-styles')) return;
        
        const style = document.createElement('style');
        style.id = 'beautiful-alert-styles';
        style.innerHTML = `
            .beautiful-alert-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                backdrop-filter: blur(8px);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 10000;
                opacity: 0;
                transition: opacity 0.3s ease;
            }

            .beautiful-alert-overlay.show {
                opacity: 1;
            }

            .beautiful-alert {
                background: var(--bg-card, #ffffff);
                border-radius: 16px;
                padding: 32px;
                max-width: 420px;
                width: 90%;
                box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
                transform: scale(0.8) translateY(20px);
                transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
                position: relative;
                overflow: hidden;
            }

            .beautiful-alert-overlay.show .beautiful-alert {
                transform: scale(1) translateY(0);
            }

            .beautiful-alert::before {
                content: '';
                position: absolute;
                top: 0;
                left: 0;
                right: 0;
                height: 4px;
                background: linear-gradient(90deg, var(--primary, #6366f1) 0%, var(--secondary, #10b981) 100%);
            }

            .beautiful-alert-icon {
                width: 60px;
                height: 60px;
                margin: 0 auto 20px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 50%;
                position: relative;
            }

            .beautiful-alert-icon.success {
                background: rgba(16, 185, 129, 0.1);
                color: #10b981;
            }

            .beautiful-alert-icon.error {
                background: rgba(239, 68, 68, 0.1);
                color: #ef4444;
            }

            .beautiful-alert-icon.warning {
                background: rgba(245, 158, 11, 0.1);
                color: #f59e0b;
            }

            .beautiful-alert-icon.info {
                background: rgba(99, 102, 241, 0.1);
                color: #6366f1;
            }

            .beautiful-alert-icon svg {
                width: 30px;
                height: 30px;
                fill: currentColor;
            }

            .beautiful-alert-icon::after {
                content: '';
                position: absolute;
                top: -5px;
                left: -5px;
                right: -5px;
                bottom: -5px;
                border-radius: 50%;
                border: 2px solid currentColor;
                opacity: 0.2;
                animation: iconPulse 2s ease-in-out infinite;
            }

            @keyframes iconPulse {
                0%, 100% { transform: scale(1); opacity: 0.2; }
                50% { transform: scale(1.1); opacity: 0.1; }
            }

            .beautiful-alert-title {
                font-size: 20px;
                font-weight: 700;
                color: var(--text-primary, #111827);
                text-align: center;
                margin-bottom: 12px;
            }

            .beautiful-alert-message {
                font-size: 15px;
                color: var(--text-secondary, #6b7280);
                text-align: center;
                line-height: 1.6;
                margin-bottom: 24px;
            }

            .beautiful-alert-buttons {
                display: flex;
                gap: 12px;
                justify-content: center;
            }

            .beautiful-alert-button {
                padding: 10px 24px;
                border: none;
                border-radius: 8px;
                font-size: 14px;
                font-weight: 600;
                cursor: pointer;
                transition: all 0.2s ease;
                position: relative;
                overflow: hidden;
            }

            .beautiful-alert-button::before {
                content: '';
                position: absolute;
                top: 50%;
                left: 50%;
                width: 0;
                height: 0;
                background: rgba(255, 255, 255, 0.2);
                border-radius: 50%;
                transform: translate(-50%, -50%);
                transition: all 0.4s ease;
            }

            .beautiful-alert-button:active::before {
                width: 200px;
                height: 200px;
            }

            .beautiful-alert-button.primary {
                background: var(--primary, #6366f1);
                color: white;
            }

            .beautiful-alert-button.primary:hover {
                background: var(--primary-dark, #4f46e5);
                transform: translateY(-2px);
                box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
            }

            .beautiful-alert-button.secondary {
                background: var(--bg-hover, #f3f4f6);
                color: var(--text-primary, #111827);
            }

            .beautiful-alert-button.secondary:hover {
                background: var(--border-color, #e5e7eb);
            }

            .beautiful-alert-button.danger {
                background: var(--danger, #ef4444);
                color: white;
            }

            .beautiful-alert-button.danger:hover {
                background: #dc2626;
                transform: translateY(-2px);
                box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
            }

            .beautiful-alert-input {
                width: 100%;
                padding: 10px 12px;
                background: var(--bg-main, #f9fafb);
                border: 2px solid var(--border-color, #e5e7eb);
                border-radius: 8px;
                font-size: 15px;
                color: var(--text-primary, #111827);
                margin-bottom: 20px;
                transition: all 0.2s ease;
            }

            .beautiful-alert-input:focus {
                outline: none;
                border-color: var(--primary, #6366f1);
                box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
            }

            /* Toast 样式 */
            .beautiful-toast {
                position: fixed;
                bottom: 20px;
                right: 20px;
                background: var(--bg-card, #ffffff);
                padding: 16px 20px;
                border-radius: 12px;
                box-shadow: 0 10px 30px rgba(0, 0, 0, 0.1);
                display: flex;
                align-items: center;
                gap: 12px;
                min-width: 300px;
                transform: translateX(400px);
                transition: transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
                z-index: 10001;
            }

            .beautiful-toast.show {
                transform: translateX(0);
            }

            .beautiful-toast-icon {
                width: 24px;
                height: 24px;
                flex-shrink: 0;
            }

            .beautiful-toast-icon.success {
                color: #10b981;
            }

            .beautiful-toast-icon.error {
                color: #ef4444;
            }

            .beautiful-toast-message {
                flex: 1;
                font-size: 14px;
                color: var(--text-primary, #111827);
                font-weight: 500;
            }

            .beautiful-toast-close {
                width: 24px;
                height: 24px;
                cursor: pointer;
                color: var(--text-secondary, #6b7280);
                transition: color 0.2s ease;
            }

            .beautiful-toast-close:hover {
                color: var(--text-primary, #111827);
            }

            @media (max-width: 480px) {
                .beautiful-alert {
                    padding: 24px;
                }

                .beautiful-toast {
                    right: 10px;
                    left: 10px;
                    min-width: unset;
                }
            }
        `;
        document.head.appendChild(style);
    }

    createContainer() {
        if (document.getElementById('beautiful-alert-container')) return;
        const container = document.createElement('div');
        container.id = 'beautiful-alert-container';
        document.body.appendChild(container);
    }

    alert(message, type = 'info', title = '') {
        return new Promise((resolve) => {
            const icons = {
                success: '<svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z"/></svg>',
                error: '<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>',
                warning: '<svg viewBox="0 0 24 24"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg>',
                info: '<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z"/></svg>'
            };

            const defaultTitles = {
                success: '成功',
                error: '错误',
                warning: '警告',
                info: '提示'
            };

            const overlay = document.createElement('div');
            overlay.className = 'beautiful-alert-overlay';
            overlay.innerHTML = `
                <div class="beautiful-alert">
                    <div class="beautiful-alert-icon ${type}">
                        ${icons[type] || icons.info}
                    </div>
                    <h3 class="beautiful-alert-title">${title || defaultTitles[type] || defaultTitles.info}</h3>
                    <p class="beautiful-alert-message">${message}</p>
                    <div class="beautiful-alert-buttons">
                        <button class="beautiful-alert-button primary">确定</button>
                    </div>
                </div>
            `;

            const container = document.getElementById('beautiful-alert-container');
            container.appendChild(overlay);

            setTimeout(() => overlay.classList.add('show'), 10);

            const button = overlay.querySelector('.beautiful-alert-button');
            button.onclick = () => {
                overlay.classList.remove('show');
                setTimeout(() => {
                    overlay.remove();
                    resolve();
                }, 300);
            };

            overlay.onclick = (e) => {
                if (e.target === overlay) {
                    button.click();
                }
            };
        });
    }

    confirm(message, title = '确认') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'beautiful-alert-overlay';
            overlay.innerHTML = `
                <div class="beautiful-alert">
                    <div class="beautiful-alert-icon warning">
                        <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>
                    </div>
                    <h3 class="beautiful-alert-title">${title}</h3>
                    <p class="beautiful-alert-message">${message}</p>
                    <div class="beautiful-alert-buttons">
                        <button class="beautiful-alert-button secondary">取消</button>
                        <button class="beautiful-alert-button primary">确定</button>
                    </div>
                </div>
            `;

            const container = document.getElementById('beautiful-alert-container');
            container.appendChild(overlay);

            setTimeout(() => overlay.classList.add('show'), 10);

            const [cancelBtn, confirmBtn] = overlay.querySelectorAll('.beautiful-alert-button');
            
            cancelBtn.onclick = () => {
                overlay.classList.remove('show');
                setTimeout(() => {
                    overlay.remove();
                    resolve(false);
                }, 300);
            };

            confirmBtn.onclick = () => {
                overlay.classList.remove('show');
                setTimeout(() => {
                    overlay.remove();
                    resolve(true);
                }, 300);
            };
        });
    }

    prompt(message, defaultValue = '', title = '输入') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'beautiful-alert-overlay';
            overlay.innerHTML = `
                <div class="beautiful-alert">
                    <div class="beautiful-alert-icon info">
                        <svg viewBox="0 0 24 24"><path d="M11 7h2v2h-2zm0 4h2v6h-2zm1-9C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z"/></svg>
                    </div>
                    <h3 class="beautiful-alert-title">${title}</h3>
                    <p class="beautiful-alert-message">${message}</p>
                    <input type="text" class="beautiful-alert-input" value="${defaultValue}" autofocus>
                    <div class="beautiful-alert-buttons">
                        <button class="beautiful-alert-button secondary">取消</button>
                        <button class="beautiful-alert-button primary">确定</button>
                    </div>
                </div>
            `;

            const container = document.getElementById('beautiful-alert-container');
            container.appendChild(overlay);

            setTimeout(() => overlay.classList.add('show'), 10);

            const input = overlay.querySelector('.beautiful-alert-input');
            const [cancelBtn, confirmBtn] = overlay.querySelectorAll('.beautiful-alert-button');
            
            input.focus();
            input.select();

            const close = (value) => {
                overlay.classList.remove('show');
                setTimeout(() => {
                    overlay.remove();
                    resolve(value);
                }, 300);
            };

            cancelBtn.onclick = () => close(null);
            confirmBtn.onclick = () => close(input.value);
            
            input.onkeypress = (e) => {
                if (e.key === 'Enter') confirmBtn.click();
                if (e.key === 'Escape') cancelBtn.click();
            };
        });
    }

    toast(message, type = 'success', duration = 3000) {
        const icons = {
            success: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z"/></svg>',
            error: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12 19 6.41z"/></svg>'
        };

        const toast = document.createElement('div');
        toast.className = 'beautiful-toast';
        toast.innerHTML = `
            <div class="beautiful-toast-icon ${type}">${icons[type] || icons.success}</div>
            <div class="beautiful-toast-message">${message}</div>
            <div class="beautiful-toast-close">
                <svg viewBox="0 0 24 24" fill="currentColor">
                    <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12 19 6.41z"/>
                </svg>
            </div>
        `;

        document.body.appendChild(toast);

        setTimeout(() => toast.classList.add('show'), 10);

        const closeToast = () => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        };

        toast.querySelector('.beautiful-toast-close').onclick = closeToast;

        if (duration > 0) {
            setTimeout(closeToast, duration);
        }
    }
}

// 创建全局实例
const beautifulAlert = new BeautifulAlert();

// 替换原生的 alert, confirm, prompt
window.alert = (message) => beautifulAlert.alert(message);
window.confirm = (message) => beautifulAlert.confirm(message);
window.prompt = (message, defaultValue) => beautifulAlert.prompt(message, defaultValue);