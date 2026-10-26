import type { Source, SourceFields } from "./src/utils/sources";

const API_URL = "/api";
let sessionPromise: Promise<unknown> | undefined;

export function setToken(): Promise<unknown> {
  if (!sessionPromise) {
    sessionPromise = fetch(`${API_URL}/set-token`, {
      method: "POST",
      credentials: "include",
    })
      .then(async (response) => {
        if (!response.ok) throw new Error("Could not establish your session");
        return response.json();
      })
      .catch((error) => {
        sessionPromise = undefined;
        throw error;
      });
  }
  return sessionPromise;
}

async function fetchWithSession(path: string, options: RequestInit = {}) {
  await setToken();
  const initialization = sessionPromise;
  let response = await fetch(`${API_URL}${path}`, {
    ...options,
    credentials: "include",
  });
  if (response.status === 401) {
    if (sessionPromise === initialization) sessionPromise = undefined;
    await setToken();
    response = await fetch(`${API_URL}${path}`, {
      ...options,
      credentials: "include",
    });
  }
  return response;
}

export async function fetchToday() {
  const response = await fetchWithSession("/today");
  if (!response.ok) throw new Error("Could not load articles");
  return response.json();
}

export async function markArticleAsRead(id: string) {
  const response = await fetchWithSession(`/articles/${id}/read`, {
    method: "POST",
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(
      typeof payload?.error === "string"
        ? payload.error
        : "Could not mark article as read",
    );
  }
}

export async function fetchInterests() {
  const response = await fetchWithSession("/interests");
  if (!response.ok) throw new Error("Could not load interests");
  return response.json();
}

export interface InterestUpdate {
  isActive: boolean;
  isMain: boolean;
}

export async function changeInterests(updates: Record<string, InterestUpdate>) {
  const response = await fetchWithSession("/change-interests", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(
      typeof payload?.error === "string"
        ? payload.error
        : "Could not save interests",
    );
  }
}

export async function fetchSources(): Promise<Source[]> {
  const response = await fetchWithSession("/sources");
  if (!response.ok) throw new Error("Could not load sources");
  return response.json();
}

async function saveSources(
  path: string,
  payload: SourceFields[] | Record<string, SourceFields>,
) {
  const response = await fetchWithSession(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => null);
    const message =
      typeof error?.error === "string" ? error.error : "Could not save sources";
    throw new Error(
      typeof error?.details === "string"
        ? `${message}: ${error.details}`
        : message,
    );
  }
}

export function insertSources(sources: SourceFields[]) {
  return saveSources("/sources", sources);
}

export function updateSources(updates: Record<string, SourceFields>) {
  return saveSources("/change-sources", updates);
}
