// Toast Notification System

function showToast(message, type = 'info', duration = 3000) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const icons = {
        success: '✓',
        error: '✗',
        warning: '⚠',
        info: 'ℹ'
    };
    const variant = typeof type === 'string' &&
        Object.prototype.hasOwnProperty.call(icons, type) ? type : 'info';
    const toast = document.createElement('div');
    toast.className = `toast ${variant}`;

    const icon = document.createElement('div');
    icon.className = 'toast-icon';
    icon.textContent = icons[variant];
    const text = document.createElement('div');
    text.className = 'toast-message';
    text.textContent = String(message);
    const close = document.createElement('div');
    close.className = 'toast-close';
    close.textContent = '×';
    close.addEventListener('click', () => toast.remove());
    toast.appendChild(icon);
    toast.appendChild(text);
    toast.appendChild(close);
    container.appendChild(toast);

    // Trigger animation
    setTimeout(() => toast.classList.add('show'), 10);

    // Auto remove after duration
    if (duration > 0) {
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }

    return toast;
}

// Convenience functions
function showSuccess(message, duration) {
    return showToast(message, 'success', duration);
}

function showError(message, duration) {
    return showToast(message, 'error', duration);
}

function showWarning(message, duration) {
    return showToast(message, 'warning', duration);
}

function showInfo(message, duration) {
    return showToast(message, 'info', duration);
}

// Replace alert() globally (optional - can be selective instead)
window.showAlert = function(message) {
    showToast(message, 'info', 4000);
};
