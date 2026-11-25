// Values the person chose to keep, per draft, shared between Review and Share.
export const keepByDraft = new Map<string, Set<string>>();
export const keepFor = (id: string) => {
  let s = keepByDraft.get(id);
  if (!s) keepByDraft.set(id, (s = new Set()));
  return s;
};
