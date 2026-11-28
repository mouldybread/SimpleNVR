// Session timeout settings. Saving restarts SimpleNVR so the new values take effect.
const SESSION_SETTINGS_URL = '/api/settings/session';
const SESSION_RESTART_URL = '/api/settings/restart';
const SESSION_SAVE_LABEL = 'Save and restart';
let sessionSettingsArmed = false;
let sessionSettingsArmTimer = null;

function sessionSettingsNode(tag, text = '', css = '') {
    const node = document.createElement(tag);
    node.textContent = String(text);
    if (css) node.style.cssText = css;
    return node;
}

function sessionSettingsStatus(text) {
    const status = document.getElementById('session-settings-status');
    if (status) status.textContent = text;
}

function sessionSettingsField(id, label, min, max) {
    const row = sessionSettingsNode('div', '', 'margin: 12px 0;');
    const title = sessionSettingsNode('label', label, 'display: block; margin-bottom: 4px;');
    title.htmlFor = id;
    const input = document.createElement('input');
    input.type = 'number'; input.id = id; input.min = String(min); input.max = String(max); input.step = '1';
    row.appendChild(title); row.appendChild(input);
    return row;
}

function disarmSessionSettings() {
    if (sessionSettingsArmTimer !== null) clearTimeout(sessionSettingsArmTimer);
    sessionSettingsArmTimer = null;
    sessionSettingsArmed = false;
    const button = document.getElementById('session-settings-save');
    if (button) button.textContent = SESSION_SAVE_LABEL;
}

function createSessionSettingsModal() {
    const modal = document.createElement('div');
    modal.id = 'session-settings-modal'; modal.className = 'modal';
    const content = document.createElement('div');
    content.className = 'modal-content';
    content.style.cssText = 'width: fit-content !important; max-width: 520px !important; height: auto !important; padding: 20px;';
    const close = sessionSettingsNode('span', '\u00d7'); close.className = 'close';
    close.addEventListener('click', closeSessionSettings);
    content.appendChild(close);
    content.appendChild(sessionSettingsNode('h2', 'Session Settings'));
    content.appendChild(sessionSettingsNode('p', 'Saving restarts SimpleNVR so the new values take effect. Live views and exports stop, recording pauses until it is back, and everyone is signed out.', 'color: #ccc; margin: 10px 0;'));
    content.appendChild(sessionSettingsField('session-idle-minutes', 'Idle timeout (minutes, 1 to 1440)', 1, 1440));
    content.appendChild(sessionSettingsField('session-lifetime-minutes', 'Maximum session length (minutes, 5 to 43200)', 5, 43200));
    const status = sessionSettingsNode('p', '', 'color: #ccc; margin: 10px 0; min-height: 1.4em;');
    status.id = 'session-settings-status'; status.setAttribute('aria-live', 'polite');
    content.appendChild(status);
    const save = sessionSettingsNode('button', SESSION_SAVE_LABEL);
    save.id = 'session-settings-save'; save.type = 'button'; save.className = 'btn btn-primary';
    save.addEventListener('click', saveSessionSettings);
    content.appendChild(save);
    modal.appendChild(content);
    modal.addEventListener('click', event => { if (event.target === modal) closeSessionSettings(); });
    document.body.appendChild(modal);
    return modal;
}

function applySessionSettings(data) {
    const keys = ['idle_minutes', 'lifetime_minutes', 'active_idle_minutes', 'active_lifetime_minutes'];
    if (!data || keys.some(key => !Number.isInteger(data[key]))) throw new Error('Unexpected response from the server');
    document.getElementById('session-idle-minutes').value = String(data.idle_minutes);
    document.getElementById('session-lifetime-minutes').value = String(data.lifetime_minutes);
    sessionSettingsStatus(data.restart_required
        ? `Running now: idle ${data.active_idle_minutes} min, maximum ${data.active_lifetime_minutes} min. The saved values apply after a restart.`
        : `Running with idle ${data.active_idle_minutes} min and maximum ${data.active_lifetime_minutes} min.`);
}

async function fetchSessionSettings() {
    sessionSettingsStatus('Loading...');
    try {
        const response = await fetch(SESSION_SETTINGS_URL, { credentials: 'same-origin', cache: 'no-store' });
        if (!response.ok) throw new Error('HTTP ' + response.status);
        applySessionSettings(await response.json());
    } catch (error) {
        sessionSettingsStatus('Unable to load session settings: ' + error.message);
    }
}

async function sessionSettingsPost(url, body) {
    const response = await fetch(url, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    const text = await response.text();
    if (!response.ok) throw new Error(text.trim() || 'HTTP ' + response.status);
    return text;
}

async function saveSessionSettings() {
    const idle = Number(document.getElementById('session-idle-minutes').value);
    const lifetime = Number(document.getElementById('session-lifetime-minutes').value);
    if (!Number.isInteger(idle) || !Number.isInteger(lifetime)) {
        sessionSettingsStatus('Enter whole numbers of minutes.');
        return;
    }
    const button = document.getElementById('session-settings-save');
    if (!sessionSettingsArmed) {
        sessionSettingsArmed = true;
        button.textContent = 'Click Again to Confirm';
        sessionSettingsStatus('This restarts SimpleNVR now. Click again within 3 seconds to continue.');
        sessionSettingsArmTimer = setTimeout(disarmSessionSettings, 3000);
        return;
    }
    disarmSessionSettings();
    button.disabled = true;
    let saved = false;
    try {
        const data = JSON.parse(await sessionSettingsPost(SESSION_SETTINGS_URL, { idle_minutes: idle, lifetime_minutes: lifetime }));
        applySessionSettings(data);
        saved = true;
        if (!data.restart_required) {
            sessionSettingsStatus('No change from the running settings, so no restart is needed.');
            button.disabled = false;
            return;
        }
        await sessionSettingsPost(SESSION_RESTART_URL, {});
        sessionSettingsStatus('Restarting... you will be returned to the sign-in page. If SimpleNVR does not come back, its container has no automatic restart policy; start it again manually.');
        if (typeof showInfo === 'function') showInfo('SimpleNVR is restarting to apply the new session settings.');
    } catch (error) {
        const message = (saved ? 'Saved, but the restart failed: ' : 'Not saved: ') + error.message;
        sessionSettingsStatus(message);
        if (typeof showError === 'function') showError(message);
        button.disabled = false;
    }
}

function openSessionSettings() {
    const modal = document.getElementById('session-settings-modal') || createSessionSettingsModal();
    modal.classList.add('active'); modal.style.display = 'block';
    fetchSessionSettings();
}

function closeSessionSettings() {
    disarmSessionSettings();
    const modal = document.getElementById('session-settings-modal');
    if (!modal) return;
    modal.classList.remove('active'); modal.style.display = 'none';
}
