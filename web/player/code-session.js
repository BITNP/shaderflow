export function createPlayerCodeSession({serverAccepted, savedDraft, template}) {
  const accepted = serverAccepted && Number.isFinite(Number(serverAccepted.version))
    ? {version: Number(serverAccepted.version), code: String(serverAccepted.code || '')}
    : null;
  const draft = savedDraft && typeof savedDraft.code === 'string' && savedDraft.code.length > 0
    ? savedDraft.code
    : accepted?.code || String(template || '');
  return {
    draft,
    activeCode: accepted?.code || '',
    acceptedVersion: accepted?.version || 0,
    nextVersionFloor: Math.max(accepted?.version || 0, Number(savedDraft?.version) || 0),
    shouldUpload: false,
  };
}
