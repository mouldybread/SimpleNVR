// Delete All Recordings: confirmation is per attempt, not deletion authority.
let deleteCode = '';
let deleteConfirmCount = 0;
let deleteRecordingsPending = false;
let deleteRecordingsVersion = 0;
let deleteConfirmTimer = null;
let deleteConfirmDeadline = 0;

function disarmRecordingDeletion() {
    deleteRecordingsVersion++;
    if (deleteConfirmTimer !== null) clearTimeout(deleteConfirmTimer);
    deleteConfirmTimer = null;
    deleteConfirmDeadline = 0;
    deleteConfirmCount = 0;
}
function resetRecordingDeletionConfirmation() {
    disarmRecordingDeletion();
    deleteCode = '';
    const input = document.getElementById('delete-code-input');
    if (input) { input.value = ''; input.disabled = deleteRecordingsPending; }
    const code = document.getElementById('delete-code');
    if (code) code.textContent = 'Reopen this dialog to confirm another attempt';
    const btn = document.getElementById('delete-recordings-btn');
    if (btn) { btn.disabled = true; btn.textContent = 'Delete All Recordings'; btn.classList.remove('confirm'); }
}
function recordingDeletionCodeValid() {
    const modal = document.getElementById('delete-recordings-modal');
    const input = document.getElementById('delete-code-input');
    const code = document.getElementById('delete-code');
    return !deleteRecordingsPending && /^[0-9]{4}$/.test(deleteCode) &&
        modal?.classList.contains('active') && input?.value === deleteCode &&
        code?.textContent === deleteCode && !!document.getElementById('delete-recordings-btn');
}
function updateRecordingDeletionInput() {
    disarmRecordingDeletion();
    const btn = document.getElementById('delete-recordings-btn');
    if (!btn) return;
    const valid = recordingDeletionCodeValid();
    btn.disabled = !valid;
    btn.textContent = deleteRecordingsPending ? 'Deleting...' : (valid ? 'Click to Confirm' : 'Delete All Recordings');
    btn.classList.remove('confirm');
}
function openDeleteAllRecordingsModal() {
    if (deleteRecordingsPending) {
        showWarning('A recording-deletion request is still pending. No additional request was started.');
        return;
    }
    resetRecordingDeletionConfirmation();
    const modal = document.getElementById('delete-recordings-modal');
    const input = document.getElementById('delete-code-input');
    const code = document.getElementById('delete-code');
    if (!modal || !input || !code || !document.getElementById('delete-recordings-btn')) return;
    deleteCode = Math.floor(1000 + Math.random() * 9000).toString();
    code.textContent = deleteCode;
    modal.classList.add('active'); modal.style.display = 'block';
    const version = deleteRecordingsVersion;
    setTimeout(() => {
        if (version === deleteRecordingsVersion && !deleteRecordingsPending && modal.classList.contains('active')) input.focus();
    }, 100);
}
function closeDeleteRecordingsModal() {
    const modal = document.getElementById('delete-recordings-modal');
    if (modal) { modal.classList.remove('active'); modal.style.display = 'none'; }
    resetRecordingDeletionConfirmation();
}
function recordingDeletionOutcome(data) {
    const counter = value => Number.isSafeInteger(value) && value >= 0;
    if (!data || Array.isArray(data) || typeof data.success !== 'boolean' ||
        !counter(data.deleted) || !counter(data.failures) ||
        !Number.isFinite(data.size_mb) || data.size_mb < 0) throw new Error('Invalid deletion response');
    const complete = data.success && data.failures === 0;
    let message = `Server reported ${data.deleted} recording(s) deleted (${data.size_mb.toFixed(2)} MB), ${data.failures} failure(s).`;
    if (!complete) message += ' Deletion incomplete. Review server state; another attempt requires reopening this dialog and fresh confirmation.';
    return {complete, deleted:data.deleted, message};
}
async function deleteAllRecordings() {
    if (deleteRecordingsPending) return;
    if (!recordingDeletionCodeValid()) { updateRecordingDeletionInput(); return; }
    const btn = document.getElementById('delete-recordings-btn');
    if (deleteConfirmCount !== 1 || Date.now() >= deleteConfirmDeadline) {
        disarmRecordingDeletion();
        deleteConfirmCount = 1;
        deleteConfirmDeadline = Date.now() + 3000;
        const version = deleteRecordingsVersion;
        btn.textContent = 'Click Again to Confirm'; btn.classList.add('confirm'); btn.disabled = false;
        deleteConfirmTimer = setTimeout(() => {
            if (version === deleteRecordingsVersion && !deleteRecordingsPending) updateRecordingDeletionInput();
        }, 3000);
        return;
    }
    deleteRecordingsPending = true;
    resetRecordingDeletionConfirmation();
    btn.textContent = 'Deleting...';
    let responseStatus = null;
    try {
        const response = await fetch('/api/recordings/delete-all', {method:'POST'});
        responseStatus = response.status;
        if (!response.ok) throw new Error('Deletion HTTP response was unsuccessful');
        const outcome = recordingDeletionOutcome(await response.json());
        if (outcome.complete) { showSuccess(outcome.message); closeDeleteRecordingsModal(); }
        else showWarning(outcome.message);
        if ((outcome.complete || outcome.deleted > 0) && typeof loadTimelineData === 'function') {
            try { loadTimelineData(); }
            catch (error) {
                console.error('Deletion result received, but timeline refresh failed:', error);
                showWarning('Deletion result received, but the timeline could not refresh. Reload the page to review it.');
            }
        }
    } catch (error) {
        console.error('Unable to verify recording-deletion outcome:', error);
        const status = Number.isInteger(responseStatus) ? ` (HTTP ${responseStatus})` : '';
        showError(`Recording-deletion outcome could not be verified${status}. Files may have been removed; no deletion totals are confirmed. Review server state before reopening this dialog for a fresh confirmation. No automatic retry was made.`);
    } finally {
        deleteRecordingsPending = false;
        resetRecordingDeletionConfirmation();
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const input = document.getElementById('delete-code-input');
    if (input) input.oninput = updateRecordingDeletionInput;
    const modal = document.getElementById('delete-recordings-modal');
    if (modal) modal.onclick = e => { if (e.target === modal) closeDeleteRecordingsModal(); };
});
