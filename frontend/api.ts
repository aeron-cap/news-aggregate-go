import type { Source, SourceFields } from './src/utils/sources'

const API_URL = "http://localhost:6767";

export async function fetchToday() {
  const response = await fetch(`${API_URL}/today`);
  if (!response.ok) throw new Error("Could not load articles")
  return response.json();
}

export async function markArticleAsRead(id: number) {
  const response = await fetch(`${API_URL}/articles/${id}/read`, {
    method: "POST",
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(typeof payload?.error === "string" ? payload.error : "Could not mark article as read");
  }
}

export async function fetchInterests() {
  const response = await fetch(`${API_URL}/interests`);
  if (!response.ok) throw new Error("Could not load interests")
  return response.json();
}

export interface InterestUpdate {
  isActive: boolean;
  isMain: boolean;
}

export async function changeInterests(updates: Record<number, InterestUpdate>) {
  const response = await fetch(`${API_URL}/change-interests`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(typeof payload?.error === "string" ? payload.error : "Could not save interests");
  }
}

export async function fetchSources(): Promise<Source[]> {
  const response = await fetch(`${API_URL}/sources`);
  if (!response.ok) throw new Error("Could not load sources");
  return response.json();
}

async function saveSources(path: string, payload: SourceFields[] | Record<number, SourceFields>) {
  const response = await fetch(`${API_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => null);
    const message = typeof error?.error === "string" ? error.error : "Could not save sources";
    throw new Error(typeof error?.details === "string" ? `${message}: ${error.details}` : message);
  }
}

export function insertSources(sources: SourceFields[]) {
  return saveSources('/sources', sources);
}

export function updateSources(updates: Record<number, SourceFields>) {
  return saveSources('/change-sources', updates);
}
